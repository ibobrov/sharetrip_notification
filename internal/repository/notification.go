package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/ibobrov/sharetrip_notification/internal/repository/entity"
	"github.com/jackc/pgx/v5"
)

type NotificationRepository struct{}

func NewNotificationRepository() *NotificationRepository {
	return &NotificationRepository{}
}

func (r *NotificationRepository) Create(
	ctx context.Context,
	tx pgx.Tx,
	notification entity.Notification,
) (entity.Notification, error) {
	row := tx.QueryRow(ctx, `
		INSERT INTO notifications (id, recipient_id, type, status, payload)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, recipient_id, type, status, payload, created_at
	`, notification.ID, notification.RecipientID, notification.Type, notification.Status, notification.Payload)

	created, err := scanNotification(row)
	if err != nil {
		return entity.Notification{}, fmt.Errorf("create notification: %w", err)
	}
	return created, nil
}

func (r *NotificationRepository) Get(
	ctx context.Context,
	tx pgx.Tx,
	id uuid.UUID,
) (entity.Notification, error) {
	row := tx.QueryRow(ctx, `
		SELECT id, recipient_id, type, status, payload, created_at
		FROM notifications
		WHERE id = $1
	`, id)

	notification, err := scanNotification(row)
	if err != nil {
		return entity.Notification{}, fmt.Errorf("get notification: %w", err)
	}
	return notification, nil
}

func scanNotification(row pgx.Row) (entity.Notification, error) {
	var notification entity.Notification
	if err := row.Scan(
		&notification.ID,
		&notification.RecipientID,
		&notification.Type,
		&notification.Status,
		&notification.Payload,
		&notification.CreatedAt,
	); err != nil {
		return entity.Notification{}, err
	}
	notification.CreatedAt = notification.CreatedAt.UTC()
	return notification, nil
}
