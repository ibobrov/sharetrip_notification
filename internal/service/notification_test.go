package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ibobrov/sharetrip_notification/internal/config"
	"github.com/ibobrov/sharetrip_notification/internal/repository"
	"github.com/ibobrov/sharetrip_notification/internal/repository/entity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestNotificationService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := serviceTestPool(t, ctx)
	repositoryBase := repository.NewNotificationRepository()
	service := NewNotificationService(pool, repositoryBase)
	input := CreateNotificationInput{
		RecipientID: "user-42",
		Type:        "trip.published",
		Payload:     []byte(`{"trip_id":"trip-7","details":{"seats":2}}`),
	}

	first, err := service.CreateNotification(ctx, input)
	require.NoError(t, err)
	second, err := service.CreateNotification(ctx, input)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, "created", first.Status)
	require.False(t, first.CreatedAt.IsZero())
	require.Equal(t, time.UTC, first.CreatedAt.Location())
	require.JSONEq(t, string(input.Payload), string(first.Payload))
	require.Equal(t, int64(2), notificationCount(t, ctx, pool))

	found, err := service.GetNotification(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, first, found)
	require.Equal(t, int64(2), notificationCount(t, ctx, pool))

	missing, err := service.GetNotification(ctx, uuid.New())
	require.ErrorIs(t, err, ErrNotificationNotFound)
	require.Equal(t, Notification{}, missing)

	databaseErr := errors.New("database read failed")
	readFailureService := NewNotificationService(pool, failingRepository{
		base:   repositoryBase,
		getErr: databaseErr,
	})
	failedRead, err := readFailureService.GetNotification(ctx, first.ID)
	require.ErrorIs(t, err, databaseErr)
	require.Equal(t, Notification{}, failedRead)
	require.Equal(t, int64(2), notificationCount(t, ctx, pool))

	operationErr := errors.New("late operation failed")
	rollbackService := NewNotificationService(pool, failingRepository{
		base:      repositoryBase,
		createErr: operationErr,
	})
	rolledBack, err := rollbackService.CreateNotification(ctx, CreateNotificationInput{
		RecipientID: "rollback-user",
		Type:        "rollback",
		Payload:     []byte(`{"value":1}`),
	})
	require.ErrorIs(t, err, operationErr)
	require.Equal(t, Notification{}, rolledBack)
	require.Equal(t, int64(2), notificationCount(t, ctx, pool))

	_, err = pool.Exec(ctx, "CREATE TABLE service_commit_guard (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		CREATE TABLE service_commit_failure (
			guard_id INTEGER REFERENCES service_commit_guard (id)
				DEFERRABLE INITIALLY DEFERRED
		)
	`)
	require.NoError(t, err)
	commitFailureService := NewNotificationService(pool, commitFailureRepository{base: repositoryBase})
	commitFailed, err := commitFailureService.CreateNotification(ctx, CreateNotificationInput{
		RecipientID: "commit-user",
		Type:        "commit",
		Payload:     []byte(`{}`),
	})
	var postgresErr *pgconn.PgError
	require.ErrorAs(t, err, &postgresErr)
	require.Equal(t, "23503", postgresErr.Code)
	require.Equal(t, Notification{}, commitFailed)
	require.Equal(t, int64(2), notificationCount(t, ctx, pool))

	beginErr := errors.New("begin failed")
	beginFailureService := NewNotificationService(beginnerFunc(func(context.Context) (pgx.Tx, error) {
		return nil, beginErr
	}), repositoryBase)
	beginFailed, err := beginFailureService.CreateNotification(ctx, input)
	require.ErrorIs(t, err, beginErr)
	require.Equal(t, Notification{}, beginFailed)

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	canceled, err := service.CreateNotification(canceledCtx, input)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, Notification{}, canceled)
	require.Equal(t, int64(2), notificationCount(t, ctx, pool))
}

type beginnerFunc func(context.Context) (pgx.Tx, error)

func (f beginnerFunc) Begin(ctx context.Context) (pgx.Tx, error) {
	return f(ctx)
}

type failingRepository struct {
	base      *repository.NotificationRepository
	createErr error
	getErr    error
}

func (r failingRepository) Create(
	ctx context.Context,
	tx pgx.Tx,
	notification entity.Notification,
) (entity.Notification, error) {
	created, err := r.base.Create(ctx, tx, notification)
	if err != nil {
		return entity.Notification{}, err
	}
	if r.createErr != nil {
		return entity.Notification{}, r.createErr
	}
	return created, nil
}

func (r failingRepository) Get(
	ctx context.Context,
	tx pgx.Tx,
	id uuid.UUID,
) (entity.Notification, error) {
	if r.getErr != nil {
		return entity.Notification{}, r.getErr
	}
	return r.base.Get(ctx, tx, id)
}

type commitFailureRepository struct {
	base *repository.NotificationRepository
}

func (r commitFailureRepository) Create(
	ctx context.Context,
	tx pgx.Tx,
	notification entity.Notification,
) (entity.Notification, error) {
	created, err := r.base.Create(ctx, tx, notification)
	if err != nil {
		return entity.Notification{}, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO service_commit_failure (guard_id) VALUES (1)"); err != nil {
		return entity.Notification{}, err
	}
	return created, nil
}

func (r commitFailureRepository) Get(
	ctx context.Context,
	tx pgx.Tx,
	id uuid.UUID,
) (entity.Notification, error) {
	return r.base.Get(ctx, tx, id)
}

func serviceTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	container, err := postgres.Run(
		ctx,
		"postgres:16.6",
		postgres.WithDatabase("notifications_test"),
		postgres.WithUsername("notifications"),
		postgres.WithPassword("notifications"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(time.Minute),
		),
	)
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)

	pool, err := repository.NewPool(ctx, config.DBConfig{
		Host:     host,
		Port:     int(port.Num()),
		Name:     "notifications_test",
		User:     "notifications",
		Password: "notifications",
		SSLMode:  "disable",
	})
	require.NoError(t, err)

	database := stdlib.OpenDBFromPool(pool)
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok)
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		database,
		os.DirFS(filepath.Join(filepath.Dir(source), "..", "..", "migrations")),
	)
	require.NoError(t, err)
	_, err = provider.Up(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, provider.Close())
		pool.Close()
		require.NoError(t, testcontainers.TerminateContainer(container))
	})

	return pool
}

func notificationCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int64 {
	t.Helper()

	var count int64
	err := pool.QueryRow(ctx, "SELECT count(*) FROM notifications").Scan(&count)
	require.NoError(t, err)
	return count
}
