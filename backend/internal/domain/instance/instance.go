// Package instance is the domain model for a MariaDB instance running on a
// registered server.
package instance

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
)

// Status is the lifecycle state of an instance.
type Status string

const (
	StatusProvisioning Status = "provisioning"
	StatusRunning      Status = "running"
	StatusStopped      Status = "stopped"
	StatusError        Status = "error"
	StatusDeleting     Status = "deleting"
)

// Instance is a MariaDB server process/container tracked by the control plane.
type Instance struct {
	ID             uuid.UUID
	ServerID       uuid.UUID
	Name           string
	ContainerID    *string
	MariaDBVersion string
	Port           int
	Status         Status
	Labels         map[string]string
	Tags           []string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int
	DeletedAt      *time.Time
}

// NewInstance validates input and builds an Instance in the running state.
// (This registers an already-running MariaDB instance; live provisioning via an
// agent is a later increment.)
func NewInstance(serverID uuid.UUID, name, mariadbVersion string, port int, labels map[string]string, tags []string) (*Instance, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 63 {
		return nil, apperr.Invalid("name", "name is required and must be at most 63 characters")
	}
	if strings.TrimSpace(mariadbVersion) == "" {
		return nil, apperr.Invalid("mariadb_version", "mariadb_version is required")
	}
	if port < 1 || port > 65535 {
		return nil, apperr.Invalid("port", "port must be between 1 and 65535")
	}
	if labels == nil {
		labels = map[string]string{}
	}
	if tags == nil {
		tags = []string{}
	}
	return &Instance{
		ID:             uuid.New(),
		ServerID:       serverID,
		Name:           name,
		MariaDBVersion: mariadbVersion,
		Port:           port,
		Status:         StatusRunning,
		Labels:         labels,
		Tags:           tags,
	}, nil
}
