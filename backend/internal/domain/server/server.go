// Package server is the domain model for managed hosts in the fleet.
// It has no dependency on transport, storage, or framework code.
package server

import (
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/TajBrains/fleetdock/backend/internal/platform/apperr"
)

// Status is the lifecycle state of a server.
type Status string

const (
	StatusPending  Status = "pending"
	StatusOnline   Status = "online"
	StatusOffline  Status = "offline"
	StatusDraining Status = "draining"
	StatusError    Status = "error"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusOnline, StatusOffline, StatusDraining, StatusError:
		return true
	}
	return false
}

// Server is the aggregate root for a host that runs a MariaDB agent.
type Server struct {
	ID              uuid.UUID
	Name            string
	Hostname        string
	Address         *string
	Status          Status
	AgentVersion    *string
	MariaDBVersion  *string
	OS              *string
	Labels          map[string]string
	Tags            []string
	LastHeartbeatAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         int
	DeletedAt       *time.Time
}

// NewServer validates input and constructs a Server in the pending state.
// Persistence-managed fields (timestamps, version) are set by the repository.
func NewServer(name, hostname string, address, osName *string, labels map[string]string, tags []string) (*Server, error) {
	name = strings.TrimSpace(name)
	hostname = strings.TrimSpace(hostname)

	if err := validateName(name); err != nil {
		return nil, err
	}
	if hostname == "" {
		return nil, apperr.Invalid("hostname", "hostname is required")
	}
	if len(hostname) > 253 {
		return nil, apperr.Invalid("hostname", "hostname must be at most 253 characters")
	}

	if labels == nil {
		labels = map[string]string{}
	}
	if tags == nil {
		tags = []string{}
	}

	return &Server{
		ID:       uuid.New(),
		Name:     name,
		Hostname: hostname,
		Address:  address,
		Status:   StatusPending,
		OS:       osName,
		Labels:   labels,
		Tags:     tags,
	}, nil
}

func validateName(name string) error {
	if name == "" {
		return apperr.Invalid("name", "name is required")
	}
	if len(name) < 2 || len(name) > 63 {
		return apperr.Invalid("name", "name must be between 2 and 63 characters")
	}
	for _, r := range name {
		if !unicode.IsLower(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return apperr.Invalid("name", "name may only contain lowercase letters, digits, '-' and '_'")
		}
	}
	return nil
}
