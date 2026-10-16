package api

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
)

func NewRouter(service notificationService, logger *slog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(requestLog(logger))

	handler := newNotificationHandler(service)
	notifications := app.Group("/api/v1/notifications")
	notifications.Post("/", handler.create)
	notifications.Get("/:id", handler.get)
	return app
}
