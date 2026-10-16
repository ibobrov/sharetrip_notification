package api

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-Id"

const (
	operationLocalKey      = "operation"
	notificationIDLocalKey = "notification_id"
	errorLocalKey          = "request_error"
)

type loggerContextKey struct{}

func requestLog(baseLogger *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		startedAt := time.Now()
		requestID := uuid.NewString()
		requestLogger := baseLogger.With(slog.String("request_id", requestID))

		c.Set(requestIDHeader, requestID)
		c.SetUserContext(context.WithValue(c.UserContext(), loggerContextKey{}, requestLogger))

		err := c.Next()
		status := responseStatus(c, err)
		attributes := []slog.Attr{
			slog.String("operation", localString(c, operationLocalKey, c.Method()+" "+c.Path())),
			slog.Int("result", status),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		}
		if notificationID := localString(c, notificationIDLocalKey, ""); notificationID != "" {
			attributes = append(attributes, slog.String("notification_id", notificationID))
		}
		if requestError := localString(c, errorLocalKey, ""); requestError != "" {
			attributes = append(attributes, slog.String("error", requestError))
		}

		requestLogger.LogAttrs(c.UserContext(), slog.LevelInfo, "request completed", attributes...)
		return err
	}
}

func responseStatus(c *fiber.Ctx, err error) int {
	if err == nil {
		return c.Response().StatusCode()
	}
	var fiberError *fiber.Error
	if errors.As(err, &fiberError) {
		return fiberError.Code
	}
	return fiber.StatusInternalServerError
}

func localString(c *fiber.Ctx, key, fallback string) string {
	value, ok := c.Locals(key).(string)
	if !ok || value == "" {
		return fallback
	}
	return value
}
