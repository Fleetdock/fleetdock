// Package backup is the domain model for database backups stored in object
// storage.
package backup

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Status is the lifecycle state of a backup.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusExpired   Status = "expired"
	StatusDeleted   Status = "deleted"
)

// Backup is one dump of a database uploaded to a destination.
type Backup struct {
	ID            uuid.UUID
	DatabaseID    uuid.UUID
	JobID         *uuid.UUID
	DestinationID *uuid.UUID
	Type          string // manual | scheduled
	Engine        string // mariadb-dump
	Status        Status
	StorageURL    *string
	SizeBytes     *int64
	Checksum      *string
	StartedAt     *time.Time
	CompletedAt   *time.Time
	Error         *string
	CreatedBy     *uuid.UUID
	CreatedAt     time.Time
	Version       int
}

// ListFilter narrows backup listings.
type ListFilter struct {
	DatabaseID *uuid.UUID
	Status     *Status
	Limit      int
	Offset     int
}

// Page is one page of backups plus the unpaginated total.
type Page struct {
	Items []*Backup
	Total int
}

// CompleteInput carries the terminal outcome of a backup run.
type CompleteInput struct {
	Status     Status
	StorageURL *string
	SizeBytes  *int64
	Checksum   *string
	Error      *string
}

// Repository is the persistence port for backups.
type Repository interface {
	Create(ctx context.Context, b *Backup) error
	GetByID(ctx context.Context, id uuid.UUID) (*Backup, error)
	List(ctx context.Context, f ListFilter) (Page, error)
	MarkRunning(ctx context.Context, id uuid.UUID) error
	Complete(ctx context.Context, id uuid.UUID, in CompleteInput) error
}
