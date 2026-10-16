package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibobrov/sharetrip_notification/internal/api"
	"github.com/ibobrov/sharetrip_notification/internal/app"
	"github.com/ibobrov/sharetrip_notification/internal/repository"
	"github.com/ibobrov/sharetrip_notification/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type notificationResponse struct {
	ID          string          `json:"id"`
	RecipientID string          `json:"recipient_id"`
	Type        string          `json:"type"`
	Status      string          `json:"status"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"created_at"`
}

func TestNotificationSurvivesApplicationRestart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	container, err := postgres.Run(
		ctx,
		"postgres:16.6",
		postgres.WithDatabase("sharetrip_notification"),
		postgres.WithUsername("notification"),
		postgres.WithPassword("notification"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(time.Minute),
		),
	)
	require.NoError(t, err)
	testcontainers.CleanupContainer(t, container)

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	applyMigrations(t, dsn)

	firstPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	firstRouter := newRouter(firstPool)

	created := createNotification(t, firstRouter)
	assertStoredNotification(t, firstPool, created)
	require.NoError(t, firstRouter.Shutdown())
	firstPool.Close()

	secondPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(secondPool.Close)
	secondRouter := newRouter(secondPool)
	t.Cleanup(func() { require.NoError(t, secondRouter.Shutdown()) })

	request := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/"+created.ID, nil)
	response, err := secondRouter.Test(request, -1)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Equal(t, http.StatusOK, response.StatusCode)

	var found notificationResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&found))
	assertNotificationEqual(t, created, found)
}

func newRouter(pool *pgxpool.Pool) *fiber.App {
	notificationRepository := repository.NewNotificationRepository()
	notificationService := service.NewNotificationService(pool, notificationRepository)
	return api.NewRouter(notificationService, app.NewLogger(io.Discard))
}

func createNotification(t *testing.T, router *fiber.App) notificationResponse {
	t.Helper()

	body := []byte(`{"recipient_id":"client-123","type":"trip_published","payload":{"trip_id":"trip-456"}}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notifications", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := router.Test(request, -1)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Equal(t, http.StatusCreated, response.StatusCode)

	var created notificationResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&created))
	require.NotEmpty(t, created.ID)
	require.Equal(t, "client-123", created.RecipientID)
	require.Equal(t, "trip_published", created.Type)
	require.Equal(t, "created", created.Status)
	require.JSONEq(t, `{"trip_id":"trip-456"}`, string(created.Payload))
	require.False(t, created.CreatedAt.IsZero())
	return created
}

func assertStoredNotification(t *testing.T, pool *pgxpool.Pool, expected notificationResponse) {
	t.Helper()

	var stored notificationResponse
	err := pool.QueryRow(context.Background(), `
		SELECT id, recipient_id, type, status, payload, created_at
		FROM notifications
		WHERE id = $1
	`, expected.ID).Scan(
		&stored.ID,
		&stored.RecipientID,
		&stored.Type,
		&stored.Status,
		&stored.Payload,
		&stored.CreatedAt,
	)
	require.NoError(t, err)
	stored.CreatedAt = stored.CreatedAt.UTC()
	assertNotificationEqual(t, expected, stored)
}

func assertNotificationEqual(t *testing.T, expected, actual notificationResponse) {
	t.Helper()

	require.Equal(t, expected.ID, actual.ID)
	require.Equal(t, expected.RecipientID, actual.RecipientID)
	require.Equal(t, expected.Type, actual.Type)
	require.Equal(t, expected.Status, actual.Status)
	require.JSONEq(t, string(expected.Payload), string(actual.Payload))
	require.Equal(t, expected.CreatedAt, actual.CreatedAt)
}

func applyMigrations(t *testing.T, dsn string) {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(db, filepath.Join("..", "..", "migrations")))
}
