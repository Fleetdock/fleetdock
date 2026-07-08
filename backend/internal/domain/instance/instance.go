// Package instance is the domain model for a database instance tracked by
// the control plane — either managed (runs on a registered server, reached
// through that server's agent) or external (an existing instance anywhere,
// reached directly by the control plane).
package instance

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
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

func newBase(name string, engine Engine, engineVersion string, port int, username *string, labels map[string]string, tags []string) (*Instance, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 63 {
		return nil, apperr.Invalid("name", "name is required and must be at most 63 characters")
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
