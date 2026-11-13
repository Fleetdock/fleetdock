package engine

import "context"

// Extensions manages PostgreSQL extensions of a database.
type Extensions interface {
	// Extensions lists the extensions the server offers, installed ones
	// with their version and schema.
	Extensions(ctx context.Context, p ConnParams, database string) ([]Extension, error)
	// InstallExtension runs CREATE EXTENSION in a schema (empty = the first
	// of the search_path), at a version (empty = the default one).
	InstallExtension(ctx context.Context, p ConnParams, database, name, schema, version string) error
	// UpdateExtension runs ALTER EXTENSION … UPDATE [TO version].
	UpdateExtension(ctx context.Context, p ConnParams, database, name, version string) error
	// DropExtension drops an extension; cascade also drops what uses it.
	DropExtension(ctx context.Context, p ConnParams, database, name string, cascade bool) error
}

// Extension is an extension the server offers.
type Extension struct {
	Name           string `json:"name"`
	DefaultVersion string `json:"default_version"`
	Comment        string `json:"comment"`
	// InstalledVersion and Schema are set when it is installed in the
	// database.
	InstalledVersion string `json:"installed_version,omitempty"`
	Schema           string `json:"schema,omitempty"`
	// Relocatable: ALTER EXTENSION … SET SCHEMA can move it.
	Relocatable bool `json:"relocatable"`
	// Trusted: a non-superuser with CREATE on the database may install it
	// (PostgreSQL 13+).
	Trusted bool `json:"trusted"`
	// Superuser: installing it needs a superuser, unless it is trusted.
	Superuser bool `json:"superuser"`
	// Versions it can be updated to from the installed one.
	Updates []string `json:"updates,omitempty"`
}
