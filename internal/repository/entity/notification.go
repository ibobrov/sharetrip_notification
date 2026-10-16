package entity

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID          uuid.UUID
	RecipientID string
	Type        string
	Status      string
	Payload     json.RawMessage
	CreatedAt   time.Time
}
