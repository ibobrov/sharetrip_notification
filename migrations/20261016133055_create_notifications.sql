-- +goose Up
CREATE TABLE notifications (
    id UUID PRIMARY KEY,
    recipient_id TEXT NOT NULL,
    type TEXT NOT NULL,
    status TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
-- Destructive: dropping the table does not preserve notification data.
DROP TABLE notifications;
