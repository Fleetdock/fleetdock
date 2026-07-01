// Package database is the domain model for a logical MariaDB database managed
// by the control plane.
package database

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
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
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusCreating, StatusActive, StatusLocked, StatusMigrating, StatusDeleting, StatusError:
		return true
	}
	return false
}

// Database is the aggregate for a managed logical database.
type Database struct {
	ID                uuid.UUID
	InstanceID        uuid.UUID
	Name              string
	Charset           string
	Collation         string
	Status            Status
	SizeBytes         int64
	ActiveConnections int
	LockedAt          *time.Time
	LockedBy          *uuid.UUID
	Labels            map[string]string
	Tags              []string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Version           int
	DeletedAt         *time.Time
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
