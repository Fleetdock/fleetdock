// Package move is the domain model for the move-database saga: back up a
// source database, restore it into a target instance/name, verify, and
// optionally drop the source.
package move

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Status is the lifecycle state of a move.
type Status string

const (
	StatusPending   Status = "pending"
	StatusBackingUp Status = "backing_up"
	StatusRestoring Status = "restoring"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Move tracks a database move as it advances through its sub-operations.
type Move struct {
	ID               uuid.UUID
	SourceDatabaseID uuid.UUID
	TargetInstanceID uuid.UUID
	TargetDatabase   string
	DestinationID    uuid.UUID
	DropSource       bool
	BackupID         *uuid.UUID
	RestoreJobID     *uuid.UUID
	Status           Status
	TableCount       *int
	Error            *string
	CreatedBy        *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Version          int
}

// Repository is the persistence port for moves.
type Repository interface {
	Create(ctx context.Context, m *Move) error
	GetByID(ctx context.Context, id uuid.UUID) (*Move, error)
	List(ctx context.Context) ([]*Move, error)
	GetByBackupID(ctx context.Context, backupID uuid.UUID) (*Move, error)
	GetByRestoreJobID(ctx context.Context, jobID uuid.UUID) (*Move, error)
	Update(ctx context.Context, m *Move) error
}
