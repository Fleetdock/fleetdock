// Package dbaccess models the Fleetdock-managed database roles used for
// interactive access (SQL console, table browser, exports, data editing).
// Each database gets at most one read-only and one read-write role, scoped to
// that database alone, so a user's reach in the engine matches their
// permission in Fleetdock instead of the instance admin's.
package dbaccess

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Mode is the access level of a console role.
type Mode string

const (
	ModeRead  Mode = "ro"
	ModeWrite Mode = "rw"
)

// Role is one provisioned console role.
type Role struct {
	DatabaseID uuid.UUID
	Mode       Mode
	Username   string
	SecretRef  string
	CreatedAt  time.Time
	AppliedAt  time.Time
}

// Repository persists console roles.
type Repository interface {
	// Get returns the role for (database, mode), or apperr NotFound.
	Get(ctx context.Context, databaseID uuid.UUID, mode Mode) (*Role, error)
	// Upsert records a role (insert or replace).
	Upsert(ctx context.Context, r *Role) error
	// Touch records that the role's grants were just (re)applied.
	Touch(ctx context.Context, databaseID uuid.UUID, mode Mode) error
	// ListByDatabase returns every role of a database.
	ListByDatabase(ctx context.Context, databaseID uuid.UUID) ([]*Role, error)
	// DeleteByDatabase forgets every role of a database.
	DeleteByDatabase(ctx context.Context, databaseID uuid.UUID) error
}
