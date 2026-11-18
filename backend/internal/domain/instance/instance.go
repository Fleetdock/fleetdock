// Package instance is the domain model for a database instance tracked by
// the control plane — either managed (runs on a registered server, reached
// through that server's agent) or external (an existing instance anywhere,
// reached directly by the control plane).
package instance

import (
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// Engine identifies the database engine.
type Engine string

const (
	EngineMariaDB    Engine = "mariadb"
	EngineMySQL      Engine = "mysql"
	EnginePostgres   Engine = "postgres"
	EngineClickHouse Engine = "clickhouse"
)

// Valid reports whether e is a supported engine.
func (e Engine) Valid() bool {
	switch e {
	case EngineMariaDB, EngineMySQL, EnginePostgres, EngineClickHouse:
		return true
	}
	return false
}

// Family groups engines whose logical dumps load into each other: "postgres",
// "clickhouse", or "mysql" for MySQL and MariaDB.
func (e Engine) Family() string {
	switch e {
	case EnginePostgres:
		return "postgres"
	case EngineClickHouse:
		return "clickhouse"
	}
	return "mysql"
}

// DumpFamily is the engine family a backup made with the given dump tool
// (as recorded on the backup) can be restored to.
func DumpFamily(tool string) string {
	switch tool {
	case "pg_dump", "postgres":
		return "postgres"
	case "clickhouse-native", "clickhouse":
		return "clickhouse"
	}
	return "mysql"
}

// FamilyLabel names an engine family for messages.
func FamilyLabel(family string) string {
	switch family {
	case "postgres":
		return "PostgreSQL"
	case "clickhouse":
		return "ClickHouse"
	}
	return "MySQL or MariaDB"
}

// DefaultPort is the engine's conventional listening port. ClickHouse is
// reached over its HTTP interface.
func (e Engine) DefaultPort() int {
	switch e {
	case EnginePostgres:
		return 5432
	case EngineClickHouse:
		return 8123
	}
	return 3306
}

// AdminUser is the engine's default superuser account name.
func (e Engine) AdminUser() string {
	switch e {
	case EnginePostgres:
		return "postgres"
	case EngineClickHouse:
		return "default"
	}
	return "root"
}

// SupportsSQLImport reports whether a .sql file can be loaded into the
// engine's databases. Fleetdock's ClickHouse backups are not SQL, and loading
// SQL scripts into ClickHouse is not supported yet.
func (e Engine) SupportsSQLImport() bool { return e != EngineClickHouse }

// SupportsBackupCompare reports whether a table can be compared row by row
// with a restored backup copy (it needs row keys, which ClickHouse lacks).
func (e Engine) SupportsBackupCompare() bool { return e != EngineClickHouse }

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
	// DataAccess is the login the table browser, SQL console, exports and
	// imports use on this instance's databases.
	DataAccess DataAccess
	// DataUsername and DataSecretRef are the dedicated login used when
	// DataAccess is "login"; both nil otherwise.
	DataUsername  *string
	DataSecretRef *string
	// TLSEnabled is set when a managed instance's engine serves TLS with a
	// certificate from the install's authority, valid until TLSExpiresAt.
	TLSEnabled   bool
	TLSExpiresAt *time.Time
}

// DataAccess selects the login used for interactive work on an instance's
// databases (table browser, SQL console, exports, imports).
type DataAccess string

const (
	// DataAccessAdmin uses the instance's admin login. It reaches every
	// database and the account tables, so only instance administrators may
	// use it.
	DataAccessAdmin DataAccess = "admin"
	// DataAccessLogin uses a dedicated login the operator configured; the
	// server's own grants decide what it can do.
	DataAccessLogin DataAccess = "login"
	// DataAccessManaged has Fleetdock create a read-only and a read-write
	// role per database, confined to that database by the server.
	DataAccessManaged DataAccess = "managed"
)

// Valid reports whether d is a supported data access mode.
func (d DataAccess) Valid() bool {
	switch d {
	case DataAccessAdmin, DataAccessLogin, DataAccessManaged:
		return true
	}
	return false
}

// DataAccessOrDefault returns the data access mode, defaulting to "admin".
func (i *Instance) DataAccessOrDefault() DataAccess {
	if i.DataAccess == "" {
		return DataAccessAdmin
	}
	return i.DataAccess
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

// TLSModeOrDefault returns the TLS mode, defaulting to DefaultTLSMode.
func (i *Instance) TLSModeOrDefault() string {
	if i.TLSMode == "" {
		return i.DefaultTLSMode()
	}
	return i.TLSMode
}

// DefaultTLSMode is the TLS mode a new instance gets when none is given:
// "prefer", which MySQL and PostgreSQL negotiate in-band. ClickHouse's HTTP
// interface cannot fall back that way — "prefer" there means plain HTTP on
// 8123 — so it defaults to "require" rather than sending the admin password
// unencrypted.
func (i *Instance) DefaultTLSMode() string {
	if i.Engine == EngineClickHouse {
		return "require"
	}
	return "prefer"
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
	if engine == EngineClickHouse {
		// The container serves plain HTTP on whatever port was picked, 8443
		// included: say so rather than let the ClickHouse default
		// ("require") or the port number suggest HTTPS.
		base.TLSMode = "disable"
	}
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
	host, err := ValidateHost(host)
	if err != nil {
		return nil, err
	}
	base, err := newBase(name, engine, engineVersion, port, username, labels, tags)
	if err != nil {
		return nil, err
	}
	base.Kind = KindExternal
	base.Host = &host
	return base, nil
}

// ValidateHost trims and checks an external instance's host, returning the
// canonical form. Only an IP address or a DNS hostname is accepted: the host
// is written into the gateway's HAProxy configuration and handed to the
// dump/restore tools, so whitespace, separators or a socket path must never
// get through.
func ValidateHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", apperr.Invalid("host", "host is required for external instances")
	}
	if !ValidHost(host) {
		return "", apperr.Invalid("host", "host must be an IP address or a hostname (letters, digits, '-', '_' and '.')")
	}
	return host, nil
}

// hostnameRe matches a DNS hostname: dot-separated labels of letters, digits,
// '-' and '_' (common in container and internal names), not starting or
// ending with '-', with an optional trailing dot.
var hostnameRe = regexp.MustCompile(`^[A-Za-z0-9_]([A-Za-z0-9_-]{0,61}[A-Za-z0-9_])?(\.[A-Za-z0-9_]([A-Za-z0-9_-]{0,61}[A-Za-z0-9_])?)*\.?$`)

// ValidHost reports whether host is an IP address (without zone) or a DNS
// hostname of at most 253 characters.
func ValidHost(host string) bool {
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Zone() == ""
	}
	return len(host) <= 253 && hostnameRe.MatchString(host)
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
