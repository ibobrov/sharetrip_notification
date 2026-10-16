package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ibobrov/sharetrip_notification/internal/repository/entity"
	"github.com/jackc/pgx/v5"
)

const notificationStatusCreated = "created"

var ErrNotificationNotFound = errors.New("notification not found")

type Notification struct {
	ID          uuid.UUID
	RecipientID string
	Type        string
	Status      string
	Payload     json.RawMessage
	CreatedAt   time.Time
}

type CreateNotificationInput struct {
	RecipientID string
	Type        string
	Payload     json.RawMessage
}

type notificationRepository interface {
	Create(context.Context, pgx.Tx, entity.Notification) (entity.Notification, error)
	Get(context.Context, pgx.Tx, uuid.UUID) (entity.Notification, error)
}

type NotificationService struct {
	db         transactionBeginner
	repository notificationRepository
}

func NewNotificationService(
	db transactionBeginner,
	repository notificationRepository,
) *NotificationService {
	return &NotificationService{db: db, repository: repository}
}

func (s *NotificationService) CreateNotification(
	ctx context.Context,
	input CreateNotificationInput,
) (Notification, error) {
	created, err := inTransaction(ctx, s.db, func(tx pgx.Tx) (entity.Notification, error) {
		notification := entity.Notification{
			ID:          uuid.New(),
			RecipientID: input.RecipientID,
			Type:        input.Type,
			Status:      notificationStatusCreated,
			Payload:     input.Payload,
		}

		result, err := s.repository.Create(ctx, tx, notification)
		if err != nil {
			return entity.Notification{}, fmt.Errorf("create notification: %w", err)
		}
		return result, nil
	})
	if err != nil {
		return Notification{}, err
	}
	return notificationFromEntity(created), nil
}

func (s *NotificationService) GetNotification(ctx context.Context, id uuid.UUID) (Notification, error) {
	found, err := inTransaction(ctx, s.db, func(tx pgx.Tx) (entity.Notification, error) {
		notification, err := s.repository.Get(ctx, tx, id)
		if err != nil {
			return entity.Notification{}, fmt.Errorf("get notification: %w", err)
		}
		return notification, nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Notification{}, ErrNotificationNotFound
	}
	if err != nil {
		return Notification{}, err
	}
	return notificationFromEntity(found), nil
}

func notificationFromEntity(notification entity.Notification) Notification {
	return Notification{
		ID:          notification.ID,
		RecipientID: notification.RecipientID,
		Type:        notification.Type,
		Status:      notification.Status,
		Payload:     notification.Payload,
		CreatedAt:   notification.CreatedAt,
	}
}
