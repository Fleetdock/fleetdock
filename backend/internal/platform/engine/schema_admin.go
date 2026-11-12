package engine

import (
	"context"
	"fmt"
	"strings"
)

// SchemaAdmin manages PostgreSQL schemas. MySQL and MariaDB call a database a
// schema and do not implement it.
type SchemaAdmin interface {
	// CreateSchema creates a schema owned by owner, or by the database's
	// owner when owner is empty.
	CreateSchema(ctx context.Context, p ConnParams, database, name, owner string) error
	// AlterSchema renames a schema, changes its owner or comment; nil fields
	// stay as they are. An empty comment removes it.
	AlterSchema(ctx context.Context, p ConnParams, database, name string, change SchemaChange) error
	// SchemaDependents counts what a schema holds and lists the objects of
	// other schemas that depend on it, so a drop can be weighed first.
	SchemaDependents(ctx context.Context, p ConnParams, database, name string) (*SchemaDependents, error)
	// DropSchema drops a schema: only when it is empty, or with cascade
	// everything in it and everything that depends on it.
	DropSchema(ctx context.Context, p ConnParams, database, name string, cascade bool) error
	// MoveObject moves an object to another schema (ALTER … SET SCHEMA).
	MoveObject(ctx context.Context, p ConnParams, database string, obj SchemaObject, target string) error
}

// SchemaObject names an object in a schema. Kind is "relation" (a table,
// view, materialized view, foreign table or sequence: which one is read from
// the catalog), "function", "procedure" or "type" (an enum, domain,
// composite or range type).
type SchemaObject struct {
	Kind   string `json:"kind"`
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

// SchemaChange is what AlterSchema changes.
type SchemaChange struct {
	Name    *string `json:"name,omitempty"`
	Owner   *string `json:"owner,omitempty"`
	Comment *string `json:"comment,omitempty"`
}

// SchemaDependents is what dropping a schema would take with it.
type SchemaDependents struct {
	Tables    int `json:"tables"`
	Views     int `json:"views"`
	Sequences int `json:"sequences"`
	Functions int `json:"functions"`
	Types     int `json:"types"`
	// External names objects in other schemas that depend on this one, such
	// as views over its tables or foreign keys into them (at most 50).
	External []string `json:"external"`
}

// Empty reports whether the schema holds nothing and nothing depends on it.
func (d *SchemaDependents) Empty() bool {
	return d.Tables+d.Views+d.Sequences+d.Functions+d.Types == 0 && len(d.External) == 0
}

// SchemaAdminFor returns the schema manager for an engine, if it has one.
func SchemaAdminFor(name string) (SchemaAdmin, bool) {
	e, err := For(name)
	if err != nil {
		return nil, false
	}
	sa, ok := e.(SchemaAdmin)
	return sa, ok
}

// systemSchema reports a schema PostgreSQL itself owns, which Fleetdock never
// creates, changes or drops.
func systemSchema(name string) bool {
	n := strings.ToLower(name)
	return n == "information_schema" || strings.HasPrefix(n, "pg_")
}

// checkUserSchema refuses system schemas and names that could break quoting.
func checkUserSchema(name string) error {
	if err := existingName("schema", name); err != nil {
		return err
	}
	if systemSchema(name) {
		return fmt.Errorf("%s is a system schema and cannot be changed", name)
	}
	return nil
}
