//go:build integration

package engine

import (
	"context"
	"strings"
	"testing"
)

// TestIntegrationPostgresSchemaPrivileges grants and revokes USAGE and
// CREATE, sets default privileges and checks they reach a new table, and sets
// the search_path of the database and of a role.
func TestIntegrationPostgresSchemaPrivileges(t *testing.T) {
	ctx := context.Background()
	root := itParams(t, "FLEETDOCK_IT_POSTGRES", "postgres")
	pg := &Postgres{}
	waitReady(t, pg, root)
	const db = "fd_it_privs"
	mustQuery(t, pg, root, "postgres", "DROP DATABASE IF EXISTS "+db, true)
	mustQuery(t, pg, root, "postgres", "CREATE DATABASE "+db, true)
	mustQuery(t, pg, root, "postgres", "DROP ROLE IF EXISTS fd_it_reader", true)
	mustQuery(t, pg, root, "postgres", "CREATE ROLE fd_it_reader", true)
	t.Cleanup(func() {
		mustQuery(t, pg, root, "postgres", "DROP DATABASE IF EXISTS "+db, true)
		mustQuery(t, pg, root, "postgres", "DROP ROLE IF EXISTS fd_it_reader", true)
	})
	mustQuery(t, pg, root, db, "CREATE SCHEMA sales", true)

	if err := pg.SetSchemaGrant(ctx, root, db, "sales", "fd_it_reader", true, true); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := pg.SetSchemaGrant(ctx, root, db, "sales", "fd_it_reader", true, false); err != nil {
		t.Fatalf("revoke create: %v", err)
	}
	if err := pg.SetSchemaGrant(ctx, root, db, "sales", root.User, false, false); err == nil {
		t.Error("changed the owner's privileges")
	}
	if err := pg.SetSchemaGrant(ctx, root, db, "sales", "nobody_here", true, false); err == nil {
		t.Error("granted to a missing role")
	}

	// The database-wide grants list reads schema privileges on every
	// supported version (information_schema.schema_privileges is 17+).
	grants, err := pg.SchemaGrants(ctx, root, db)
	if err != nil {
		t.Fatalf("schema grants: %v", err)
	}
	found := false
	for _, g := range grants {
		if g.User == "fd_it_reader" && strings.Contains(strings.Join(g.Privileges, ","), "USAGE") {
			found = true
		}
	}
	if !found {
		t.Errorf("schema grants = %+v", grants)
	}

	change := DefaultPrivilegeChange{ObjectType: "tables", Grantee: "fd_it_reader", Privileges: []string{"SELECT"}}
	if err := pg.AlterDefaultPrivileges(ctx, root, db, "sales", change); err != nil {
		t.Fatalf("default privileges: %v", err)
	}
	mustQuery(t, pg, root, db, "CREATE TABLE sales.orders (id int)", true)
	res := mustQuery(t, pg, root, db, `SELECT has_table_privilege('fd_it_reader', 'sales.orders', 'SELECT')`, false)
	if v := *res.Rows[0][0]; v != "true" && v != "t" {
		t.Errorf("the new table did not get SELECT: %s", v)
	}

	acc, err := pg.SchemaAccess(ctx, root, db, "sales")
	if err != nil {
		t.Fatal(err)
	}
	if acc.Owner != root.User || len(acc.Grants) != 1 || acc.Grants[0] != (SchemaRoleGrant{Role: "fd_it_reader", Usage: true}) {
		t.Errorf("access = %+v", acc)
	}
	if len(acc.Defaults) != 1 || acc.Defaults[0].ObjectType != "tables" || acc.Defaults[0].Grantee != "fd_it_reader" ||
		strings.Join(acc.Defaults[0].Privileges, ",") != "SELECT" {
		t.Errorf("defaults = %+v", acc.Defaults)
	}

	change.Revoke = true
	if err := pg.AlterDefaultPrivileges(ctx, root, db, "sales", change); err != nil {
		t.Fatalf("revoke default privileges: %v", err)
	}
	if acc, _ := pg.SchemaAccess(ctx, root, db, "sales"); len(acc.Defaults) != 0 {
		t.Errorf("defaults after revoke = %+v", acc.Defaults)
	}

	if err := pg.SetSearchPath(ctx, root, db, "", []string{"sales", "public"}); err != nil {
		t.Fatalf("database search_path: %v", err)
	}
	if err := pg.SetSearchPath(ctx, root, db, "fd_it_reader", []string{"$user", "sales"}); err != nil {
		t.Fatalf("role search_path: %v", err)
	}
	paths, err := pg.SearchPaths(ctx, root, db)
	if err != nil {
		t.Fatal(err)
	}
	if paths.Database != "sales, public" || len(paths.Roles) != 1 || paths.Roles[0].Path != `"$user", sales` || paths.Server == "" {
		t.Errorf("paths = %+v", paths)
	}
	// A new session in the database starts with it.
	if res := mustQuery(t, pg, root, db, "SHOW search_path", false); *res.Rows[0][0] != "sales, public" {
		t.Errorf("session search_path = %s", *res.Rows[0][0])
	}
	if err := pg.SetSearchPath(ctx, root, db, "", nil); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if paths, _ := pg.SearchPaths(ctx, root, db); paths.Database != "" {
		t.Errorf("after reset: %+v", paths)
	}
	// Without a setting orders is not found; a session started with its own
	// search_path resolves names there.
	if _, err := pg.Query(ctx, root, db, "SELECT count(*) FROM orders", 1, false); err == nil {
		t.Error("orders resolved without a search_path")
	}
	session := root
	session.SearchPath = []string{"sales"}
	if res := mustQuery(t, pg, session, db, "SELECT count(*) FROM orders", false); *res.Rows[0][0] != "0" {
		t.Errorf("orders through the session search_path = %s", *res.Rows[0][0])
	}
}
