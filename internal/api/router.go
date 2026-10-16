package api

import (
	"errors"

	"github.com/gofiber/fiber/v2"
)

const maxRequestBodySize = 65_536

func NewRouter(service notificationService) *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          errorHandler,
	})
	app.Use(func(c *fiber.Ctx) error {
		if len(c.Body()) > maxRequestBodySize {
			return writeError(c, fiber.StatusRequestEntityTooLarge, "request body is too large")
		}
		return c.Next()
	})

	handler := newNotificationHandler(service)
	notifications := app.Group("/api/v1/notifications")
	notifications.Post("/", handler.create)
	notifications.Get("/:id", handler.get)
	notifications.All("/", methodNotAllowed("POST"))
	notifications.All("/:id", methodNotAllowed("GET"))

	app.Use(func(c *fiber.Ctx) error {
		return writeError(c, fiber.StatusNotFound, "route not found")
	})

	return app
}

func methodNotAllowed(allow string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderAllow, allow)
		return writeError(c, fiber.StatusMethodNotAllowed, "method not allowed")
	}
}

func errorHandler(c *fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	message := "internal server error"

	var fiberError *fiber.Error
	if errors.As(err, &fiberError) {
		status = fiberError.Code
		switch status {
		case fiber.StatusRequestEntityTooLarge:
			message = "request body is too large"
		case fiber.StatusNotFound:
			message = "route not found"
		case fiber.StatusMethodNotAllowed:
			message = "method not allowed"
		}
	}

	return writeError(c, status, message)
}
