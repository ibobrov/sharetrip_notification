package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/asaskevich/govalidator/v12"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/ibobrov/sharetrip_notification/internal/service"
)

type notificationService interface {
	CreateNotification(context.Context, service.CreateNotificationInput) (service.Notification, error)
	GetNotification(context.Context, uuid.UUID) (service.Notification, error)
}

type notificationHandler struct {
	service notificationService
}

type createNotificationRequest struct {
	RecipientID string          `json:"recipient_id" valid:"required"`
	Type        string          `json:"type" valid:"required"`
	Payload     json.RawMessage `json:"payload" valid:"-"`
}

type getNotificationRequest struct {
	ID string `valid:"required,uuid"`
}

type notificationResponse struct {
	ID          uuid.UUID       `json:"id"`
	RecipientID string          `json:"recipient_id"`
	Type        string          `json:"type"`
	Status      string          `json:"status"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"created_at"`
}

func newNotificationHandler(service notificationService) *notificationHandler {
	return &notificationHandler{service: service}
}

func (h *notificationHandler) create(c *fiber.Ctx) error {
	c.Locals(operationLocalKey, "create_notification")

	var request createNotificationRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "некорректное тело запроса")
	}
	if _, err := govalidator.ValidateStruct(request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if len(request.Payload) == 0 {
		request.Payload = json.RawMessage(`{}`)
	}

	created, err := h.service.CreateNotification(c.UserContext(), service.CreateNotificationInput{
		RecipientID: request.RecipientID,
		Type:        request.Type,
		Payload:     request.Payload,
	})
	if err != nil {
		c.Locals(errorLocalKey, "service_error")
		return fiber.ErrInternalServerError
	}
	c.Locals(notificationIDLocalKey, created.ID.String())
	return c.Status(fiber.StatusCreated).JSON(notificationFromService(created))
}

func (h *notificationHandler) get(c *fiber.Ctx) error {
	c.Locals(operationLocalKey, "get_notification")

	request := getNotificationRequest{ID: c.Params("id")}
	if _, err := govalidator.ValidateStruct(request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	id, err := uuid.Parse(request.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	c.Locals(notificationIDLocalKey, id.String())

	found, err := h.service.GetNotification(c.UserContext(), id)
	if errors.Is(err, service.ErrNotificationNotFound) {
		return fiber.NewError(fiber.StatusNotFound, "notification not found")
	}
	if err != nil {
		c.Locals(errorLocalKey, "service_error")
		return fiber.ErrInternalServerError
	}
	return c.Status(fiber.StatusOK).JSON(notificationFromService(found))
}

func notificationFromService(notification service.Notification) notificationResponse {
	return notificationResponse{
		ID:          notification.ID,
		RecipientID: notification.RecipientID,
		Type:        notification.Type,
		Status:      notification.Status,
		Payload:     notification.Payload,
		CreatedAt:   notification.CreatedAt.UTC(),
	}
}
