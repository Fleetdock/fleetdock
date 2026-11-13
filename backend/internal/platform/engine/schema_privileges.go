package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// SchemaPrivileges reads and changes who may use a PostgreSQL schema, the
// privileges new objects in it are created with, and the search_path of a
// database and its roles.
type SchemaPrivileges interface {
	// SchemaAccess reports a schema's owner, the roles with USAGE or CREATE
	// on it and its default privileges.
	SchemaAccess(ctx context.Context, p ConnParams, database, schema string) (*SchemaAccess, error)
	// SetSchemaGrant grants or revokes USAGE and CREATE on a schema so that
	// the role ends up with exactly the given ones.
	SetSchemaGrant(ctx context.Context, p ConnParams, database, schema, role string, usage, create bool) error
	// AlterDefaultPrivileges grants or revokes privileges on the objects a
	// role creates in a schema from now on.
	AlterDefaultPrivileges(ctx context.Context, p ConnParams, database, schema string, change DefaultPrivilegeChange) error
	// SearchPaths lists the search_path set for the database and for roles
	// in it.
	SearchPaths(ctx context.Context, p ConnParams, database string) (*SearchPaths, error)
	// SetSearchPath sets the search_path of the database (role "") or of a
	// role in it; no schemas resets it.
	SetSearchPath(ctx context.Context, p ConnParams, database, role string, schemas []string) error
}

// SchemaAccess is who may use a schema.
type SchemaAccess struct {
	Schema string `json:"schema"`
	Owner  string `json:"owner"`
	// Grants are the roles other than the owner with USAGE or CREATE;
	// "PUBLIC" is every role.
	Grants   []SchemaRoleGrant  `json:"grants"`
	Defaults []DefaultPrivilege `json:"default_privileges"`
	// Roles are the roles that could be granted something, for pickers.
	Roles []string `json:"roles"`
}

// SchemaRoleGrant is what one role may do with a schema.
type SchemaRoleGrant struct {
	Role   string `json:"role"`
	Usage  bool   `json:"usage"`
	Create bool   `json:"create"`
}

// DefaultPrivilege is one grant that objects created in a schema get.
type DefaultPrivilege struct {
	// Creator is the role whose new objects get the grant.
	Creator string `json:"creator"`
	// ObjectType is tables, sequences, functions or types.
	ObjectType string   `json:"object_type"`
	Grantee    string   `json:"grantee"`
	Privileges []string `json:"privileges"`
}

// DefaultPrivilegeChange is one ALTER DEFAULT PRIVILEGES.
type DefaultPrivilegeChange struct {
	// Creator is the role whose future objects it covers; empty is the
	// login running the change.
	Creator    string   `json:"creator"`
	ObjectType string   `json:"object_type"`
	Grantee    string   `json:"grantee"`
	Privileges []string `json:"privileges"`
	Revoke     bool     `json:"revoke"`
}

// SearchPaths is the search_path of a database and of roles in it.
type SearchPaths struct {
	// Database is the database's own setting; empty when it has none.
	Database string `json:"database"`
	// Server is what a session gets without any setting.
	Server string           `json:"server"`
	Roles  []RoleSearchPath `json:"roles"`
}

// RoleSearchPath is a role's search_path in one database.
type RoleSearchPath struct {
	Role string `json:"role"`
	Path string `json:"path"`
}

// SchemaPrivilegesFor returns the schema privilege manager for an engine, if
// it has one.
func SchemaPrivilegesFor(name string) (SchemaPrivileges, bool) {
	e, err := For(name)
	if err != nil {
		return nil, false
	}
	sp, ok := e.(SchemaPrivileges)
	return sp, ok
}

// defaultPrivilegeKinds maps an object type to its pg_default_acl code and
// the privileges it accepts.
var defaultPrivilegeKinds = map[string]struct {
	code  string
	privs []string
}{
	"tables":    {"r", []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"}},
	"sequences": {"S", []string{"USAGE", "SELECT", "UPDATE"}},
	"functions": {"f", []string{"EXECUTE"}},
	"types":     {"T", []string{"USAGE"}},
}

// checkDefaultPrivileges validates a change's object type and privileges and
// returns the privileges upper-cased, deduplicated, in catalog order.
func checkDefaultPrivileges(objectType string, privs []string) ([]string, error) {
	kind, ok := defaultPrivilegeKinds[objectType]
	if !ok {
		return nil, fmt.Errorf("object type must be tables, sequences, functions or types")
	}
	want := map[string]bool{}
	for _, p := range privs {
		want[strings.ToUpper(strings.TrimSpace(p))] = true
	}
	var out []string
	for _, p := range kind.privs {
		if want[p] {
			out = append(out, p)
			delete(want, p)
		}
	}
	if want["ALL"] {
		delete(want, "ALL")
		out = kind.privs
	}
	if len(want) > 0 {
		bad := make([]string, 0, len(want))
		for p := range want {
			bad = append(bad, p)
		}
		sort.Strings(bad)
		return nil, fmt.Errorf("%s is not a privilege on %s (use %s)", strings.Join(bad, ", "), objectType, strings.Join(kind.privs, ", "))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("choose at least one privilege")
	}
	return out, nil
}

// SearchPathSQL builds the SET statement for a session's search_path. The
// names are those of existing schemas and are quoted, so they are taken
// literally.
func SearchPathSQL(schemas []string) (string, error) {
	list, err := searchPathList(schemas)
	if err != nil {
		return "", err
	}
	return "SET search_path TO " + list, nil
}

func searchPathList(schemas []string) (string, error) {
	if len(schemas) == 0 {
		return "", fmt.Errorf("choose at least one schema")
	}
	if len(schemas) > 32 {
		return "", fmt.Errorf("a search_path takes at most 32 schemas")
	}
	q := make([]string, len(schemas))
	for i, s := range schemas {
		if err := existingName("schema", s); err != nil {
			return "", err
		}
		q[i] = quotePGIdent(s)
	}
	return strings.Join(q, ", "), nil
}

// granteeSQL quotes a grantee; PUBLIC is the keyword.
func granteeSQL(role string) string {
	if strings.EqualFold(role, "public") {
		return "PUBLIC"
	}
	return quotePGIdent(role)
}
