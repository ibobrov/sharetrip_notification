package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ibobrov/sharetrip_notification/internal/config"
	"github.com/ibobrov/sharetrip_notification/internal/repository"
	"github.com/ibobrov/sharetrip_notification/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestNotificationAPI(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := apiTestPool(t, ctx)
	notificationRepository := repository.NewNotificationRepository()
	notificationService := service.NewNotificationService(pool, notificationRepository)
	app := NewRouter(notificationService)
	t.Cleanup(func() {
		require.NoError(t, app.Shutdown())
	})

	validBody := `{
		"recipient_id":" Client-42 ",
		"type":"Trip.Published",
		"payload":{"trip_id":"trip-7","number":9007199254740993,"items":[null,true]}
	}`
	createdResult := performRequest(t, app, http.MethodPost, "/api/v1/notifications", validBody, "application/json; charset=utf-8")
	require.Equal(t, http.StatusCreated, createdResult.status)
	require.Contains(t, createdResult.header.Get("Content-Type"), "application/json")
	createdEnvelope := decodeEnvelope(t, createdResult.body)
	require.NotNil(t, createdEnvelope.Errors)
	require.Empty(t, createdEnvelope.Errors)
	require.NotNil(t, createdEnvelope.Data)
	require.Equal(t, " Client-42 ", createdEnvelope.Data.RecipientID)
	require.Equal(t, "Trip.Published", createdEnvelope.Data.Type)
	require.Equal(t, "created", createdEnvelope.Data.Status)
	require.False(t, createdEnvelope.Data.CreatedAt.IsZero())
	require.Equal(t, time.UTC, createdEnvelope.Data.CreatedAt.Location())
	require.Contains(t, string(createdEnvelope.Data.Payload), "9007199254740993")
	require.Equal(t, "/api/v1/notifications/"+createdEnvelope.Data.ID.String(), createdResult.header.Get("Location"))
	require.Equal(t, int64(1), apiNotificationCount(t, ctx, pool))

	getResult := performRequest(
		t,
		app,
		http.MethodGet,
		"/api/v1/notifications/"+strings.ToUpper(createdEnvelope.Data.ID.String()),
		"",
		"",
	)
	require.Equal(t, http.StatusOK, getResult.status)
	getEnvelope := decodeEnvelope(t, getResult.body)
	require.Equal(t, createdEnvelope, getEnvelope)
	require.Equal(t, int64(1), apiNotificationCount(t, ctx, pool))

	defaultPayloadResult := performRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/notifications",
		`{"recipient_id":"default","type":"default"}`,
		"application/json",
	)
	require.Equal(t, http.StatusCreated, defaultPayloadResult.status)
	defaultPayloadEnvelope := decodeEnvelope(t, defaultPayloadResult.body)
	require.JSONEq(t, `{}`, string(defaultPayloadEnvelope.Data.Payload))

	invalidBodies := []string{
		`{}`,
		`{"recipient_id":null,"type":"type"}`,
		`{"recipient_id":1,"type":"type"}`,
		`{"recipient_id":"","type":"type"}`,
		`{"recipient_id":"\u2000","type":"type"}`,
		`{"recipient_id":"recipient"}`,
		`{"recipient_id":"recipient","type":null}`,
		`{"recipient_id":"recipient","type":false}`,
		`{"recipient_id":"recipient","type":" "}`,
		`{"recipient_id":"recipient","type":"type","payload":null}`,
		`{"recipient_id":"recipient","type":"type","payload":[]}`,
		`{"recipient_id":"recipient","type":"type","payload":"value"}`,
		`{"recipient_id":"recipient","type":"type","payload":1}`,
		`{"recipient_id":"recipient","type":"type","payload":true}`,
		`{"recipient_id":"recipient","type":"type","id":"` + uuid.NewString() + `"}`,
		`{"recipient_id":"recipient","type":"type","status":"created"}`,
		`{"recipient_id":"recipient","type":"type","created_at":"2026-10-16T00:00:00Z"}`,
		`{"recipient_id":"recipient","type":"type","unknown":true}`,
		`{"recipient_id":"recipient","type":`,
		`[]`,
		`null`,
		`{"recipient_id":"recipient","type":"type"} {}`,
		``,
	}
	countBeforeInvalid := apiNotificationCount(t, ctx, pool)
	for _, body := range invalidBodies {
		result := performRequest(t, app, http.MethodPost, "/api/v1/notifications", body, "application/json")
		assertErrorResult(t, result, http.StatusBadRequest)
	}
	require.Equal(t, countBeforeInvalid, apiNotificationCount(t, ctx, pool))

	for _, contentType := range []string{"", "text/plain", "application/json; charset="} {
		result := performRequest(t, app, http.MethodPost, "/api/v1/notifications", validBody, contentType)
		assertErrorResult(t, result, http.StatusUnsupportedMediaType)
	}
	require.Equal(t, countBeforeInvalid, apiNotificationCount(t, ctx, pool))

	boundaryBody := bodyWithSize(t, maxRequestBodySize)
	boundaryResult := performRequest(t, app, http.MethodPost, "/api/v1/notifications", boundaryBody, "application/json")
	require.Equal(t, http.StatusCreated, boundaryResult.status)
	overLimitResult := performRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/notifications",
		bodyWithSize(t, maxRequestBodySize+1),
		"application/json",
	)
	assertErrorResult(t, overLimitResult, http.StatusRequestEntityTooLarge)

	badIDResult := performRequest(t, app, http.MethodGet, "/api/v1/notifications/not-a-uuid", "", "")
	assertErrorResult(t, badIDResult, http.StatusBadRequest)
	nonCanonicalIDResult := performRequest(
		t,
		app,
		http.MethodGet,
		"/api/v1/notifications/"+strings.ReplaceAll(uuid.NewString(), "-", ""),
		"",
		"",
	)
	assertErrorResult(t, nonCanonicalIDResult, http.StatusBadRequest)
	missingResult := performRequest(t, app, http.MethodGet, "/api/v1/notifications/"+uuid.NewString(), "", "")
	assertErrorResult(t, missingResult, http.StatusNotFound)
	zeroIDResult := performRequest(t, app, http.MethodGet, "/api/v1/notifications/"+uuid.Nil.String(), "", "")
	assertErrorResult(t, zeroIDResult, http.StatusNotFound)

	methodResult := performRequest(t, app, http.MethodPut, "/api/v1/notifications", "", "")
	assertErrorResult(t, methodResult, http.StatusMethodNotAllowed)
	require.Equal(t, "POST", methodResult.header.Get("Allow"))
	unknownRouteResult := performRequest(t, app, http.MethodGet, "/api/v1/unknown", "", "")
	assertErrorResult(t, unknownRouteResult, http.StatusNotFound)

	failureText := "database host and SQL must stay private"
	failingApp := NewRouter(stubNotificationService{
		createErr: errors.New(failureText),
		getErr:    errors.New(failureText),
	})
	postFailure := performRequest(t, failingApp, http.MethodPost, "/api/v1/notifications", validBody, "application/json")
	assertInternalError(t, postFailure, failureText)
	getFailure := performRequest(t, failingApp, http.MethodGet, "/api/v1/notifications/"+uuid.NewString(), "", "")
	assertInternalError(t, getFailure, failureText)

	countBeforeInvalidJSONB := apiNotificationCount(t, ctx, pool)
	invalidJSONBResult := performRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/notifications",
		`{"recipient_id":"jsonb","type":"overflow","payload":{"number":1e1000000}}`,
		"application/json",
	)
	assertErrorResult(t, invalidJSONBResult, http.StatusBadRequest)
	require.Equal(t, countBeforeInvalidJSONB, apiNotificationCount(t, ctx, pool))

	parallelBody := `{"recipient_id":"parallel","type":"same","payload":{"value":1}}`
	countBeforeParallel := apiNotificationCount(t, ctx, pool)
	parallelResults := make([]requestResult, 2)
	parallelErrors := make([]error, 2)
	var waitGroup sync.WaitGroup
	for index := range parallelResults {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			parallelResults[index], parallelErrors[index] = performRequestRaw(
				app,
				http.MethodPost,
				"/api/v1/notifications",
				parallelBody,
				"application/json",
			)
		}()
	}
	waitGroup.Wait()
	require.NoError(t, parallelErrors[0])
	require.NoError(t, parallelErrors[1])
	firstParallel := decodeEnvelope(t, parallelResults[0].body)
	secondParallel := decodeEnvelope(t, parallelResults[1].body)
	require.Equal(t, http.StatusCreated, parallelResults[0].status)
	require.Equal(t, http.StatusCreated, parallelResults[1].status)
	require.NotEqual(t, firstParallel.Data.ID, secondParallel.Data.ID)
	require.Equal(t, countBeforeParallel+2, apiNotificationCount(t, ctx, pool))
}

type stubNotificationService struct {
	createErr error
	getErr    error
}

func (s stubNotificationService) CreateNotification(
	context.Context,
	service.CreateNotificationInput,
) (service.Notification, error) {
	return service.Notification{}, s.createErr
}

func (s stubNotificationService) GetNotification(context.Context, uuid.UUID) (service.Notification, error) {
	return service.Notification{}, s.getErr
}

type requestResult struct {
	status int
	header http.Header
	body   []byte
}

type testEnvelope struct {
	Data   *notificationResponse `json:"data"`
	Errors []string              `json:"errors"`
}

func performRequest(
	t *testing.T,
	app interface {
		Test(*http.Request, ...int) (*http.Response, error)
	},
	method string,
	path string,
	body string,
	contentType string,
) requestResult {
	t.Helper()

	result, err := performRequestRaw(app, method, path, body, contentType)
	require.NoError(t, err)
	return result
}

func performRequestRaw(
	app interface {
		Test(*http.Request, ...int) (*http.Response, error)
	},
	method string,
	path string,
	body string,
	contentType string,
) (requestResult, error) {
	request, err := http.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	if err != nil {
		return requestResult{}, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := app.Test(request, -1)
	if err != nil {
		return requestResult{}, err
	}
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		_ = response.Body.Close()
		return requestResult{}, err
	}
	if err := response.Body.Close(); err != nil {
		return requestResult{}, err
	}
	return requestResult{status: response.StatusCode, header: response.Header, body: responseBody}, nil
}

func decodeEnvelope(t *testing.T, body []byte) testEnvelope {
	t.Helper()

	var response testEnvelope
	require.NoError(t, json.Unmarshal(body, &response))
	return response
}

func assertErrorResult(t *testing.T, result requestResult, status int) {
	t.Helper()

	require.Equal(t, status, result.status, string(result.body))
	require.Contains(t, result.header.Get("Content-Type"), "application/json")
	response := decodeEnvelope(t, result.body)
	require.Nil(t, response.Data)
	require.NotEmpty(t, response.Errors)
}

func assertInternalError(t *testing.T, result requestResult, privateText string) {
	t.Helper()

	assertErrorResult(t, result, http.StatusInternalServerError)
	response := decodeEnvelope(t, result.body)
	require.Equal(t, []string{"internal server error"}, response.Errors)
	require.NotContains(t, string(result.body), privateText)
}

func bodyWithSize(t *testing.T, size int) string {
	t.Helper()

	prefix := `{"recipient_id":"boundary","type":"boundary","payload":{"padding":"`
	suffix := `"}}`
	require.GreaterOrEqual(t, size, len(prefix)+len(suffix))
	body := prefix + strings.Repeat("a", size-len(prefix)-len(suffix)) + suffix
	require.Len(t, []byte(body), size)
	return body
}

func apiTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
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

func apiNotificationCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int64 {
	t.Helper()

	var count int64
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM notifications").Scan(&count))
	return count
}
