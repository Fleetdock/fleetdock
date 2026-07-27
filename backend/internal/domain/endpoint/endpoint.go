// Package endpoint is the domain model for database connectivity endpoints.
package endpoint

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// AccessType distinguishes private vs public connectivity.
type AccessType string

const (
	AccessPrivate AccessType = "private"
	AccessPublic  AccessType = "public"
)

// Status is the lifecycle state of an endpoint.
type Status string

const (
	StatusPending   Status = "pending"
	StatusActive    Status = "active"
	StatusDisabling Status = "disabling"
	StatusDisabled  Status = "disabled"
	StatusError     Status = "error"
)

// Protocol is the client-facing database protocol.
type Protocol string

const (
	ProtocolPostgreSQL Protocol = "postgresql"
	ProtocolMySQL      Protocol = "mysql"
	ProtocolMariaDB    Protocol = "mariadb"
)

// TLSMode is the desired TLS behavior for clients.
type TLSMode string

const (
	TLSRequired  TLSMode = "required"
	TLSPreferred TLSMode = "preferred"
	TLSDisabled  TLSMode = "disabled"
)

// TLSStatus reports the observed TLS capability.
type TLSStatus string

const (
	TLSStatusRequired      TLSStatus = "required"
	TLSStatusPreferred     TLSStatus = "preferred"
	TLSStatusDisabled      TLSStatus = "disabled"
	TLSStatusUnsupported   TLSStatus = "unsupported"
	TLSStatusMisconfigured TLSStatus = "misconfigured"
	TLSStatusUnknown       TLSStatus = "unknown"
)

// Endpoint is a connectivity target for a database.
type Endpoint struct {
	ID             uuid.UUID
	DatabaseID     uuid.UUID
	AccessType     AccessType
	Status         Status
	Protocol       Protocol
	ExternalHost   string
	ExternalPort   *int
	InternalHost   string
	InternalPort   int
	TLSMode        TLSMode
	TLSStatus      TLSStatus
	AllowedCIDRs   []string
	MaxConnections *int
	LastError      *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DisabledAt     *time.Time
	Version        int
}

// Target is where a client should actually connect, resolved from an endpoint's
// access type. Host and port must be chosen together — pairing a public host
// with a private port yields a URL that silently fails to connect.
type Target struct {
	Host     string
	Port     int
	Protocol Protocol
	TLSMode  TLSMode
}

// Target resolves the client-facing address for this endpoint.
func (e *Endpoint) Target() Target {
	t := Target{
		Host:     e.InternalHost,
		Port:     e.InternalPort,
		Protocol: e.Protocol,
		TLSMode:  e.TLSMode,
	}
	if e.AccessType == AccessPublic && e.ExternalPort != nil {
		t.Host = e.ExternalHost
		t.Port = *e.ExternalPort
	}
	return t
}

// ProtocolForEngine maps an instance engine string to a client protocol.
func ProtocolForEngine(engine string) (Protocol, error) {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "postgres":
		return ProtocolPostgreSQL, nil
	case "mysql":
		return ProtocolMySQL, nil
	case "mariadb":
		return ProtocolMariaDB, nil
	default:
		return "", apperr.Invalid("engine", "unsupported engine for connectivity")
	}
}

// NewPrivate builds a private endpoint record.
func NewPrivate(databaseID uuid.UUID, protocol Protocol, host string, port int) (*Endpoint, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, apperr.Invalid("host", "internal host is required")
	}
	if port < 1 || port > 65535 {
		return nil, apperr.Invalid("port", "port must be between 1 and 65535")
	}
	return &Endpoint{
		ID:           uuid.New(),
		DatabaseID:   databaseID,
		AccessType:   AccessPrivate,
		Status:       StatusActive,
		Protocol:     protocol,
		ExternalHost: host,
		InternalHost: host,
		InternalPort: port,
		TLSMode:      TLSPreferred,
		TLSStatus:    TLSStatusUnknown,
		AllowedCIDRs: []string{},
	}, nil
}

// NewPublicPending builds a public endpoint awaiting gateway reconciliation.
func NewPublicPending(databaseID uuid.UUID, protocol Protocol, externalHost string, externalPort int,
	internalHost string, internalPort int, allowedCIDRs []string, tlsMode TLSMode) (*Endpoint, error) {
	externalHost = strings.TrimSpace(externalHost)
	internalHost = strings.TrimSpace(internalHost)
	if externalHost == "" {
		return nil, apperr.Invalid("external_host", "external host is required")
	}
	if internalHost == "" {
		return nil, apperr.Invalid("internal_host", "internal host is required")
	}
	if externalPort < 1 || externalPort > 65535 {
		return nil, apperr.Invalid("external_port", "external port must be between 1 and 65535")
	}
	if internalPort < 1 || internalPort > 65535 {
		return nil, apperr.Invalid("internal_port", "internal port must be between 1 and 65535")
	}
	if !tlsMode.Valid() {
		return nil, apperr.Invalid("tls_mode", "invalid tls mode")
	}
	cidrs, err := NormalizeCIDRs(allowedCIDRs)
	if err != nil {
		return nil, err
	}
	return &Endpoint{
		ID:           uuid.New(),
		DatabaseID:   databaseID,
		AccessType:   AccessPublic,
		Status:       StatusPending,
		Protocol:     protocol,
		ExternalHost: externalHost,
		ExternalPort: &externalPort,
		InternalHost: internalHost,
		InternalPort: internalPort,
		TLSMode:      tlsMode,
		TLSStatus:    TLSStatusUnknown,
		AllowedCIDRs: cidrs,
	}, nil
}

func (m TLSMode) Valid() bool {
	switch m {
	case TLSRequired, TLSPreferred, TLSDisabled:
		return true
	}
	return false
}

// CanTransition reports whether moving from current to next is allowed.
func (s Status) CanTransition(next Status) bool {
	switch s {
	case StatusPending:
		return next == StatusActive || next == StatusError || next == StatusDisabling || next == StatusDisabled
	case StatusActive:
		return next == StatusDisabling || next == StatusError || next == StatusPending
	case StatusDisabling:
		return next == StatusDisabled || next == StatusError || next == StatusActive
	case StatusError:
		return next == StatusPending || next == StatusDisabling || next == StatusDisabled || next == StatusActive
	case StatusDisabled:
		return next == StatusPending
	}
	return false
}
