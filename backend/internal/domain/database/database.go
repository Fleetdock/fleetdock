// Package database is the domain model for a logical MariaDB database managed
// by the control plane.
package database

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// Status is the lifecycle state of a database.
type Status string

const (
	StatusCreating  Status = "creating"
	StatusActive    Status = "active"
	StatusLocked    Status = "locked"
	StatusMigrating Status = "migrating"
	StatusDeleting  Status = "deleting"
	StatusError     Status = "error"
	// StatusMissing: the database was not found on its server by recent
	// probes (dropped or renamed outside Fleetdock). Never deleted
	// automatically; it returns to active if it reappears.
	StatusMissing Status = "missing"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusCreating, StatusActive, StatusLocked, StatusMigrating, StatusDeleting, StatusError:
		return true
	}
	return false
}

// InstanceRef is a read-only summary of the owning instance, denormalized onto
// a Database by the queries that join it. It exists so API consumers can render
// "which instance is this on?" without a second, separately paginated request —
// the previous client-side lookup silently failed past the first page of
// instances.
//
// It is populated by List and GetByID only; write paths leave it nil.
type InstanceRef struct {
	ID       uuid.UUID
	Name     string
	Engine   string
	Kind     string
	ServerID *uuid.UUID
	// Provisioned mirrors instance.Provisioned(): a container the control plane
	// launched. Drives whether removal can offer to delete the data volume.
	Provisioned bool
	// HasCredentials mirrors instance.HasCredentials(). The UI gates physical
	// drops, live browsing and grants on it.
	HasCredentials bool
}

// Database is the aggregate for a managed logical database.
type Database struct {
	ID uuid.UUID
	// InstanceID is the owning instance. Instance, when non-nil, carries a
	// summary of that instance for display.
	InstanceID uuid.UUID
	Instance   *InstanceRef
	Name       string
	Charset    string
	Collation  string
	Status     Status
	// System marks an engine-owned database discovered by import (PostgreSQL's
	// maintenance database, MySQL's mysql/sys). It can be browsed and backed
	// up, but Delete refuses it.
	System            bool
	SizeBytes         int64
	ActiveConnections int
	LockedAt          *time.Time
	// MissingSince is when the database went missing (status "missing").
	MissingSince *time.Time
	LockedBy     *uuid.UUID
	Labels       map[string]string
	Tags         []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int
	DeletedAt    *time.Time
}

// NewDatabase validates input and builds a Database record in the active state.
func NewDatabase(instanceID uuid.UUID, name, charset, collation string, labels map[string]string, tags []string) (*Database, error) {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return nil, err
	}
	if charset == "" {
		charset = "utf8mb4"
	}
	if collation == "" {
		collation = "utf8mb4_unicode_ci"
	}
	if labels == nil {
		labels = map[string]string{}
	}
	if tags == nil {
		tags = []string{}
	}
	return &Database{
		ID:         uuid.New(),
		InstanceID: instanceID,
		Name:       name,
		Charset:    charset,
		Collation:  collation,
		Status:     StatusActive,
		Labels:     labels,
		Tags:       tags,
	}, nil
}

func validateName(name string) error {
	if name == "" {
		return apperr.Invalid("name", "name is required")
	}
	if len(name) > 64 {
		return apperr.Invalid("name", "name must be at most 64 characters")
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '$'
		if !ok {
			return apperr.Invalid("name", "name may only contain letters, digits, '_' and '$'")
		}
	}
	return nil
}

// Observed is one database as seen on its server by a probe.
type Observed struct {
	Name        string
	Charset     string
	Collation   string
	System      bool
	SizeBytes   int64
	Connections int
}

// ReconcileResult reports what a reconciliation changed.
type ReconcileResult struct {
	Added      []string
	Missing    []string // newly marked missing
	Reappeared []string
}

// MissingAfter is how long a database may go unseen before it is marked
// missing — a few probe intervals, so one failed listing does not flap it.
const MissingAfter = 3 * time.Minute

// RediscoverAfter is how long a database removed from Fleetdock (but still on
// its server) is left alone by discovery, matching the recovery window.
const RediscoverAfter = 7 * 24 * time.Hour
