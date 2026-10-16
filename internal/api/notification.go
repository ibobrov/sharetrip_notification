package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/ibobrov/sharetrip_notification/internal/service"
	"github.com/jackc/pgx/v5/pgconn"
)

const requestTimeout = 2 * time.Second

type notificationService interface {
	CreateNotification(context.Context, service.CreateNotificationInput) (service.Notification, error)
	GetNotification(context.Context, uuid.UUID) (service.Notification, error)
}

type notificationHandler struct {
	service notificationService
}

type createNotificationRequest struct {
	RecipientID json.RawMessage `json:"recipient_id"`
	Type        json.RawMessage `json:"type"`
	Payload     json.RawMessage `json:"payload"`
}

type notificationResponse struct {
	ID          uuid.UUID       `json:"id"`
	RecipientID string          `json:"recipient_id"`
	Type        string          `json:"type"`
	Status      string          `json:"status"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"created_at"`
}

type envelope struct {
	Data   any      `json:"data"`
	Errors []string `json:"errors"`
}

func newNotificationHandler(service notificationService) *notificationHandler {
	return &notificationHandler{service: service}
}

func (h *notificationHandler) create(c *fiber.Ctx) error {
	if !isJSONContentType(c.Get(fiber.HeaderContentType)) {
		return writeError(c, fiber.StatusUnsupportedMediaType, "content type must be application/json")
	}

	request, validationErr := decodeCreateRequest(c.Body())
	if validationErr != "" {
		return writeError(c, fiber.StatusBadRequest, validationErr)
	}

	ctx, cancel := context.WithTimeout(c.UserContext(), requestTimeout)
	defer cancel()
	created, err := h.service.CreateNotification(ctx, service.CreateNotificationInput{
		RecipientID: request.recipientID,
		Type:        request.notificationType,
		Payload:     request.payload,
	})
	if err != nil {
		if isInvalidJSONB(err) {
			return writeError(c, fiber.StatusBadRequest, "payload cannot be stored as JSON")
		}
		return fmt.Errorf("create notification: %w", err)
	}

	c.Location("/api/v1/notifications/" + created.ID.String())
	return c.Status(fiber.StatusCreated).JSON(successEnvelope(created))
}

func (h *notificationHandler) get(c *fiber.Ctx) error {
	id, err := parseCanonicalUUID(c.Params("id"))
	if err != nil {
		return writeError(c, fiber.StatusBadRequest, "id must be a canonical UUID")
	}

	ctx, cancel := context.WithTimeout(c.UserContext(), requestTimeout)
	defer cancel()
	found, err := h.service.GetNotification(ctx, id)
	if errors.Is(err, service.ErrNotificationNotFound) {
		return writeError(c, fiber.StatusNotFound, "notification not found")
	}
	if err != nil {
		return fmt.Errorf("get notification: %w", err)
	}

	return c.Status(fiber.StatusOK).JSON(successEnvelope(found))
}

type validatedCreateRequest struct {
	recipientID      string
	notificationType string
	payload          json.RawMessage
}

func decodeCreateRequest(body []byte) (validatedCreateRequest, string) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	var request createNotificationRequest
	if err := decoder.Decode(&request); err != nil {
		return validatedCreateRequest{}, "body must be a JSON object with known fields"
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return validatedCreateRequest{}, "body must contain exactly one JSON object"
	}

	recipientID, errMessage := requiredString(request.RecipientID, "recipient_id")
	if errMessage != "" {
		return validatedCreateRequest{}, errMessage
	}
	notificationType, errMessage := requiredString(request.Type, "type")
	if errMessage != "" {
		return validatedCreateRequest{}, errMessage
	}
	payload, errMessage := objectPayload(request.Payload)
	if errMessage != "" {
		return validatedCreateRequest{}, errMessage
	}

	return validatedCreateRequest{
		recipientID:      recipientID,
		notificationType: notificationType,
		payload:          payload,
	}, ""
}

func requiredString(raw json.RawMessage, name string) (string, string) {
	if len(raw) == 0 {
		return "", name + " is required"
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", name + " must be a non-empty string"
	}
	if strings.TrimSpace(value) == "" {
		return "", name + " must be a non-empty string"
	}
	return value, ""
}

func objectPayload(raw json.RawMessage) (json.RawMessage, string) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), ""
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, "payload must be a JSON object"
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return nil, "payload must be a JSON object"
	}
	return append(json.RawMessage(nil), trimmed...), ""
}

func isJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && strings.EqualFold(mediaType, fiber.MIMEApplicationJSON)
}

func parseCanonicalUUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || !strings.EqualFold(id.String(), value) {
		return uuid.Nil, errors.New("invalid canonical UUID")
	}
	return id, nil
}

func isInvalidJSONB(err error) bool {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return false
	}
	return postgresError.Code == "22P02" || postgresError.Code == "22003"
}

func successEnvelope(notification service.Notification) envelope {
	return envelope{
		Data: notificationResponse{
			ID:          notification.ID,
			RecipientID: notification.RecipientID,
			Type:        notification.Type,
			Status:      notification.Status,
			Payload:     notification.Payload,
			CreatedAt:   notification.CreatedAt.UTC(),
		},
		Errors: []string{},
	}
}

func writeError(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(envelope{Data: nil, Errors: []string{message}})
}
