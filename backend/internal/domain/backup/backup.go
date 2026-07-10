// Package backup is the domain model for database backups stored in object
// storage.
package backup

import (
	"context"
	"time"

	"github.com/google/uuid"

	authz "github.com/TajBrains/fleetdock/backend/internal/domain/authz"
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
	ScheduleID    *uuid.UUID
	DestinationID *uuid.UUID
	Type          string // manual | scheduled
	Engine        string // mariadb-dump
	Status        Status
	StorageURL    *string
	SizeBytes     *int64
	Checksum      *string
	StartedAt     *time.Time
	CompletedAt   *time.Time
	ExpiresAt     *time.Time
	Error         *string
	CreatedBy     *uuid.UUID
	CreatedAt     time.Time
	Version       int
}

// Expired is a completed backup past its retention boundary, carrying the
// stored object location so the retention worker can delete it.
type Expired struct {
	ID            uuid.UUID
	DestinationID uuid.UUID
	StorageURL    string
}

// ListFilter narrows backup listings.
type ListFilter struct {
	DatabaseID *uuid.UUID
	Status     *Status
	Limit      int
	Offset     int
	// Scope, when non-nil, restricts results to the caller's readable scope.
	Scope *authz.ReadSet
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
	// ListExpired returns completed backups whose expires_at has passed.
	ListExpired(ctx context.Context, now time.Time, limit int) ([]Expired, error)
	// MarkExpired flags a backup as expired after its object is deleted.
	MarkExpired(ctx context.Context, id uuid.UUID) error
	// CountByStatusSince counts backups grouped by status created since t.
	CountByStatusSince(ctx context.Context, since time.Time) (map[Status]int, error)
}
