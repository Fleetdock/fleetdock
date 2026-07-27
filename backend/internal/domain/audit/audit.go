// Package audit is the domain model for append-only audit events.
package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Event is one immutable audit record.
type Event struct {
	ID           uuid.UUID
	ActorUserID  *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	Metadata     json.RawMessage
	CreatedAt    time.Time
}

// Repository persists audit events.
type Repository interface {
	Append(ctx context.Context, e *Event) error
}
