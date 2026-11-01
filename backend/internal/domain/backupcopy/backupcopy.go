// Package backupcopy models a backup restored into a temporary database next
// to the database it came from, kept for a while so a table can be compared
// with it and rows put back ("data time machine").
package backupcopy

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Status is a copy's lifecycle state.
type Status string

const (
	StatusRestoring Status = "restoring"
	StatusReady     Status = "ready"
	StatusFailed    Status = "failed"
	// StatusDropping: the temporary database is being (or must be) dropped.
	StatusDropping Status = "dropping"
	StatusDropped  Status = "dropped"
)

// Limits.
const (
	DefaultTTL = 2 * time.Hour
	MaxTTL     = 24 * time.Hour
	// MaxDropAttempts bounds retries of dropping the temporary database.
	MaxDropAttempts = 5
)

// Copy is one backup copy.
type Copy struct {
	ID           uuid.UUID     `json:"id"`
	DatabaseID   uuid.UUID     `json:"database_id"`
	BackupID     uuid.UUID     `json:"backup_id"`
	InstanceID   uuid.UUID     `json:"instance_id"`
	ScratchName  string        `json:"scratch_name"`
	Status       Status        `json:"status"`
	JobID        *uuid.UUID    `json:"job_id,omitempty"`
	DropJobID    *uuid.UUID    `json:"-"`
	DropAttempts int           `json:"-"`
	Error        *string       `json:"error,omitempty"`
	TTL          time.Duration `json:"-"`
	ReadyAt      *time.Time    `json:"ready_at,omitempty"`
	ExpiresAt    *time.Time    `json:"expires_at,omitempty"`
	CreatedBy    *uuid.UUID    `json:"created_by,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	// BackupCreatedAt is when the backup was taken (filled on reads).
	BackupCreatedAt *time.Time `json:"backup_created_at,omitempty"`
}

// Live reports whether the copy is restoring or usable.
func (c *Copy) Live() bool { return c.Status == StatusRestoring || c.Status == StatusReady }

// Repository persists copies.
type Repository interface {
	// Create inserts a restoring copy; a live copy of the same database
	// makes it fail with a conflict.
	Create(ctx context.Context, c *Copy) error
	Get(ctx context.Context, id uuid.UUID) (*Copy, error)
	ListByDatabase(ctx context.Context, databaseID uuid.UUID) ([]Copy, error)
	SetJob(ctx context.Context, id, jobID uuid.UUID) error
	// MarkReady makes a restoring copy usable until expiresAt.
	MarkReady(ctx context.Context, id uuid.UUID, readyAt, expiresAt time.Time) error
	MarkFailed(ctx context.Context, id uuid.UUID, msg string) error
	// MarkDropping records a drop request (dropJobID nil: not enqueued yet).
	MarkDropping(ctx context.Context, id uuid.UUID, dropJobID *uuid.UUID) error
	MarkDropped(ctx context.Context, id uuid.UUID) error
	SetExpiry(ctx context.Context, id uuid.UUID, expiresAt time.Time) error
	// ListDue returns copies to drop: ready and expired, failed, or dropping
	// with no progress since staleBefore and attempts left.
	ListDue(ctx context.Context, now, staleBefore time.Time) ([]Copy, error)
}
