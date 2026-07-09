package database

import (
	"context"

	"github.com/google/uuid"

	authz "github.com/TajBrains/db-manager/backend/internal/domain/authz"
)

// ListFilter narrows a List query.
type ListFilter struct {
	InstanceID *uuid.UUID
	Status     *Status
	Search     string
	Limit      int
	Offset     int
	// Scope, when non-nil, restricts results to the caller's readable scope.
	Scope *authz.ReadSet
}

// Page is a slice of databases plus the total matching count.
type Page struct {
	Items []*Database
	Total int
}

// Repository is the persistence port for databases.
type Repository interface {
	Create(ctx context.Context, d *Database) error
	GetByID(ctx context.Context, id uuid.UUID) (*Database, error)
	List(ctx context.Context, f ListFilter) (Page, error)
	// Lock/Unlock return the updated record.
	Lock(ctx context.Context, id, lockedBy uuid.UUID) (*Database, error)
	Unlock(ctx context.Context, id uuid.UUID) (*Database, error)
	// SoftDelete marks the database deleted and opens the recovery window.
	SoftDelete(ctx context.Context, id uuid.UUID) error
	// SetStatus transitions the lifecycle status (operations engine).
	SetStatus(ctx context.Context, id uuid.UUID, status Status) error
}
