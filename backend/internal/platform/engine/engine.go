// Package engine abstracts database-engine-specific operations behind a
// small interface so the control plane and agent stay engine-agnostic.
// MariaDB/MySQL and PostgreSQL register Client implementations; live
// administration (Admin) is implemented per engine in *_admin.go.
package engine

import (
	"context"
	"fmt"
)

// ConnParams are the network + credential parameters for reaching an instance.
type ConnParams struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database,omitempty"`
}

// DatabaseInfo describes a logical database discovered on an instance.
type DatabaseInfo struct {
	Name      string `json:"name"`
	Charset   string `json:"charset"`
	Collation string `json:"collation"`
	// System marks an engine-owned database (the PostgreSQL maintenance
	// database, MySQL's mysql/sys schemas). These are imported so they can be
	// browsed and backed up, but must never be dropped.
	System bool `json:"system,omitempty"`
}

// Client is the set of engine operations the control plane / agent needs.
type Client interface {
	// Ping verifies connectivity and returns the server version.
	Ping(ctx context.Context, p ConnParams) (string, error)
	// ListDatabases returns non-system databases with charset/collation.
	ListDatabases(ctx context.Context, p ConnParams) ([]DatabaseInfo, error)
	CreateDatabase(ctx context.Context, p ConnParams, name, charset, collation string) error
	DropDatabase(ctx context.Context, p ConnParams, name string) error
	// CountTables returns the number of user tables in a database (used to
	// verify a restore produced a non-empty schema).
	CountTables(ctx context.Context, p ConnParams, database string) (int, error)
	// DumpArgs returns the argv (binary candidates + args) and extra env vars
	// for a logical dump of one database, writing SQL to stdout.
	DumpArgs(p ConnParams, database string) (binaries []string, args []string, env []string)
	// RestoreArgs returns the argv and extra env for restoring a SQL stream
	// from stdin.
	RestoreArgs(p ConnParams, database string) (binaries []string, args []string, env []string)
}

// systemDatabases are the engine-owned databases that ListDatabases reports
// with System set. They are importable (browsable, backup-able) but the
// control plane refuses to drop them: the PostgreSQL maintenance database is
// what every admin connection targets, and MySQL's mysql/sys schemas hold the
// account and metadata catalogue. Purely virtual schemas
// (information_schema, performance_schema) are not listed at all — they cannot
// be dumped, so importing them would only produce backups that always fail.
var systemDatabases = map[string]map[string]bool{
	"postgres": {"postgres": true},
	"mysql":    {"mysql": true, "sys": true},
}

// IsSystemDatabase reports whether name is an engine-owned database that must
// not be dropped. engineName accepts any registered engine ("mariadb" and
// "mysql" share one catalogue).
func IsSystemDatabase(engineName, name string) bool {
	family := engineName
	if family == "mariadb" {
		family = "mysql"
	}
	return systemDatabases[family][name]
}

func systemDatabase(engineName, name string) bool { return IsSystemDatabase(engineName, name) }

var registry = map[string]Client{}

// Register adds an engine implementation (called from engine impls' init).
func Register(name string, c Client) { registry[name] = c }

// For returns the client registered for the engine name.
func For(name string) (Client, error) {
	c, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("engine: unsupported engine %q", name)
	}
	return c, nil
}
