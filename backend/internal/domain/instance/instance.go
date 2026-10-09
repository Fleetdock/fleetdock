// Package instance is the domain model for a database instance tracked by
// the control plane — either managed (runs on a registered server, reached
// through that server's agent) or external (an existing instance anywhere,
// reached directly by the control plane).
package instance

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// Engine identifies the database engine.
type Engine string

const (
	EngineMariaDB  Engine = "mariadb"
	EngineMySQL    Engine = "mysql"
	EnginePostgres Engine = "postgres"
)

// Valid reports whether e is a supported engine.
func (e Engine) Valid() bool {
	switch e {
	case EngineMariaDB, EngineMySQL, EnginePostgres:
		return true
	}
	return false
}

// DefaultPort is the engine's conventional listening port.
func (e Engine) DefaultPort() int {
	if e == EnginePostgres {
		return 5432
	}
	return 3306
}

// AdminUser is the engine's default superuser account name.
func (e Engine) AdminUser() string {
	if e == EnginePostgres {
		return "postgres"
	}
	return "root"
}

// Kind distinguishes managed from external instances.
type Kind string

const (
	KindManaged  Kind = "managed"
	KindExternal Kind = "external"
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

// Instance is a database server process tracked by the control plane.
type Instance struct {
	ID            uuid.UUID
	ServerID      *uuid.UUID // nil for external instances
	Name          string
	Engine        Engine
	Kind          Kind
	Host          *string // external instances only
	Port          int
	Username      *string // admin user for SQL operations
	RootSecretRef *string // ref into secrets for the admin password
	ContainerID   *string
	EngineVersion string
	Status        Status
	Labels        map[string]string
	Tags          []string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Version       int
	DeletedAt     *time.Time
	// TLSMode is how the control plane secures connections to the instance:
	// disable | prefer | require | verify-full.
	TLSMode string
	// Health is the latest probe result (nil until first probed).
	Health *Health
	// SSH, when set, routes every connection through an SSH bastion
	// (external instances only). Host is then resolved on the bastion.
	SSH *SSHTunnel
}

// SSHAuth is how the control plane authenticates to an SSH bastion.
type SSHAuth string

const (
	SSHAuthPassword SSHAuth = "password"
	SSHAuthKey      SSHAuth = "key"
)

// Valid reports whether a is a supported SSH auth method.
func (a SSHAuth) Valid() bool { return a == SSHAuthPassword || a == SSHAuthKey }

// SSHTunnel is the bastion an external instance is reached through.
type SSHTunnel struct {
	Host string
	Port int
	User string
	Auth SSHAuth
	// SecretRef points at the SSH password or private key (+ passphrase).
	SecretRef *string
	// HostKey is the bastion's pinned host key in authorized_keys format;
	// empty until the first successful connection pins it (TOFU).
	HostKey string
}

// HealthStatus is the outcome of a probe.
type HealthStatus string

const (
	HealthHealthy     HealthStatus = "healthy"
	HealthUnreachable HealthStatus = "unreachable"
	// HealthUnknown: the control plane cannot reach the instance directly
	// (typical for managed instances whose port is only open to the agent),
	// which says nothing about whether it is up.
	HealthUnknown HealthStatus = "unknown"
)

// Health is the latest result of probing an instance.
type Health struct {
	Status    HealthStatus `json:"status"`
	Version   string       `json:"version,omitempty"`
	LatencyMS int64        `json:"latency_ms"`
	Error     string       `json:"error,omitempty"`
	CheckedAt time.Time    `json:"checked_at"`
}

// TLSModeOrDefault returns the TLS mode, defaulting to "prefer".
func (i *Instance) TLSModeOrDefault() string {
	if i.TLSMode == "" {
		return "prefer"
	}
	return i.TLSMode
}

// HasCredentials reports whether SQL-level operations are possible.
func (i *Instance) HasCredentials() bool {
	return i.Username != nil && *i.Username != "" && i.RootSecretRef != nil
}

// Provisioned reports whether this instance is a container the control plane
// launched (as opposed to a pre-existing DB the user registered).
func (i *Instance) Provisioned() bool {
	return i.Kind == KindManaged && i.ContainerID != nil && *i.ContainerID != ""
}

// ContainerName is the deterministic Docker container/volume name for a
// provisioned instance.
func (i *Instance) ContainerName() string { return "dbm-" + i.ID.String() }

// NewProvisioned builds a managed instance that the agent will create as a
// Docker container. It starts in the provisioning state with a root admin user.
func NewProvisioned(serverID uuid.UUID, name string, engine Engine, engineVersion string, port int) (*Instance, error) {
	if engine == "" {
		engine = EngineMariaDB
	}
	admin := engine.AdminUser()
	base, err := newBase(name, engine, engineVersion, port, &admin, nil, nil)
	if err != nil {
		return nil, err
	}
	base.ServerID = &serverID
	base.Kind = KindManaged
	base.Status = StatusProvisioning
	return base, nil
}

// NewManaged registers an already-running instance on a managed server.
func NewManaged(serverID uuid.UUID, name string, engine Engine, engineVersion string, port int, username *string, labels map[string]string, tags []string) (*Instance, error) {
	base, err := newBase(name, engine, engineVersion, port, username, labels, tags)
	if err != nil {
		return nil, err
	}
	base.ServerID = &serverID
	base.Kind = KindManaged
	return base, nil
}

// NewExternal registers an instance the control plane reaches directly.
func NewExternal(name string, engine Engine, engineVersion, host string, port int, username *string, labels map[string]string, tags []string) (*Instance, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, apperr.Invalid("host", "host is required for external instances")
	}
	base, err := newBase(name, engine, engineVersion, port, username, labels, tags)
	if err != nil {
		return nil, err
	}
	base.Kind = KindExternal
	base.Host = &host
	return base, nil
}

// ValidateName trims and checks an instance name, returning the canonical
// form. Shared by registration and update so both enforce the same rule.
func ValidateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 63 {
		return "", apperr.Invalid("name", "name is required and must be at most 63 characters")
	}
	return name, nil
}

// ValidatePort checks a TCP port is in range.
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return apperr.Invalid("port", "port must be between 1 and 65535")
	}
	return nil
}

func newBase(name string, engine Engine, engineVersion string, port int, username *string, labels map[string]string, tags []string) (*Instance, error) {
	name, err := ValidateName(name)
	if err != nil {
		return nil, err
	}
	if engine == "" {
		engine = EngineMariaDB
	}
	if !engine.Valid() {
		return nil, apperr.Invalid("engine", "unsupported engine (supported: mariadb, mysql, postgres)")
	}
	if strings.TrimSpace(engineVersion) == "" {
		return nil, apperr.Invalid("engine_version", "engine_version is required")
	}
	if err := ValidatePort(port); err != nil {
		return nil, err
	}
	if labels == nil {
		labels = map[string]string{}
	}
	if tags == nil {
		tags = []string{}
	}
	return &Instance{
		ID:            uuid.New(),
		Name:          name,
		Engine:        engine,
		EngineVersion: engineVersion,
		Port:          port,
		Username:      username,
		Status:        StatusRunning,
		Labels:        labels,
		Tags:          tags,
	}, nil
}
