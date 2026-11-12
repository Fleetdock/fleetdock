//go:build integration

package engine

import (
	"context"
	"strings"
	"testing"
)

// TestIntegrationPostgresSchemaAdmin creates, renames, re-owns, comments and
// drops a schema, and checks what a drop would take with it.
func TestIntegrationPostgresSchemaAdmin(t *testing.T) {
	ctx := context.Background()
	root := itParams(t, "FLEETDOCK_IT_POSTGRES", "postgres")
	pg := &Postgres{}
	waitReady(t, pg, root)
	const db = "fd_it_schemas"
	mustQuery(t, pg, root, "postgres", "DROP DATABASE IF EXISTS "+db, true)
	mustQuery(t, pg, root, "postgres", "CREATE DATABASE "+db, true)
	mustQuery(t, pg, root, "postgres", "DROP ROLE IF EXISTS fd_it_schema_owner", true)
	mustQuery(t, pg, root, "postgres", "CREATE ROLE fd_it_schema_owner", true)
	t.Cleanup(func() {
		mustQuery(t, pg, root, "postgres", "DROP DATABASE IF EXISTS "+db, true)
		mustQuery(t, pg, root, "postgres", "DROP ROLE IF EXISTS fd_it_schema_owner", true)
	})

	scalar := func(sql string) string {
		res := mustQuery(t, pg, root, db, sql, false)
		if len(res.Rows) == 0 || res.Rows[0][0] == nil {
			return ""
		}
		return *res.Rows[0][0]
	}
	owner := func(schema string) string {
		return scalar(`SELECT pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname = '` + schema + `'`)
	}

	if err := pg.CreateSchema(ctx, root, db, "staging", ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	if o := owner("staging"); o != root.User {
		t.Errorf("default owner = %q, want the database owner %q", o, root.User)
	}
	if err := pg.CreateSchema(ctx, root, db, "staging", ""); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("duplicate create: %v", err)
	}
	if err := pg.CreateSchema(ctx, root, db, "pg_mine", ""); err == nil {
		t.Error("created a pg_ schema")
	}
	if err := pg.CreateSchema(ctx, root, db, "x; DROP DATABASE postgres", ""); err == nil {
		t.Error("accepted an invalid name")
	}
	if err := pg.CreateSchema(ctx, root, db, "other", "no_such_role"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("unknown owner: %v", err)
	}

	newName, newOwner, comment := "landing", "fd_it_schema_owner", "loads land here"
	if err := pg.AlterSchema(ctx, root, db, "staging", SchemaChange{Name: &newName, Owner: &newOwner, Comment: &comment}); err != nil {
		t.Fatalf("alter: %v", err)
	}
	counts, err := pg.SchemaCounts(ctx, root, db)
	if err != nil {
		t.Fatal(err)
	}
	var landing *SchemaCount
	for i := range counts {
		if counts[i].Name == "landing" {
			landing = &counts[i]
		}
	}
	if landing == nil || landing.Owner != newOwner || landing.Comment != comment {
		t.Fatalf("after alter: %+v", counts)
	}
	empty := ""
	if err := pg.AlterSchema(ctx, root, db, "landing", SchemaChange{Comment: &empty}); err != nil {
		t.Fatalf("clear comment: %v", err)
	}
	catalogName := "pg_catalog"
	if err := pg.AlterSchema(ctx, root, db, catalogName, SchemaChange{Comment: &comment}); err == nil {
		t.Error("altered pg_catalog")
	}

	// A table, a view and a function in the schema; a view and a foreign key
	// elsewhere that depend on it.
	for _, stmt := range []string{
		"CREATE TABLE landing.raw (id int PRIMARY KEY, body text)",
		"CREATE VIEW landing.recent AS SELECT * FROM landing.raw",
		"CREATE FUNCTION landing.one() RETURNS int LANGUAGE sql AS 'SELECT 1'",
		"CREATE VIEW public.raw_ids AS SELECT id FROM landing.raw",
		"CREATE TABLE public.refs (raw_id int REFERENCES landing.raw (id))",
	} {
		mustQuery(t, pg, root, db, stmt, true)
	}
	d, err := pg.SchemaDependents(ctx, root, db, "landing")
	if err != nil {
		t.Fatal(err)
	}
	if d.Tables != 1 || d.Views != 1 || d.Functions != 1 || d.Empty() {
		t.Errorf("dependents = %+v", d)
	}
	ext := strings.Join(d.External, "|")
	if !strings.Contains(ext, "view public.raw_ids") || !strings.Contains(ext, "foreign key refs_raw_id_fkey on public.refs") {
		t.Errorf("external dependents = %v", d.External)
	}

	if err := pg.DropSchema(ctx, root, db, "landing", false); err == nil {
		t.Fatal("dropped a schema that is not empty without cascade")
	}
	if err := pg.DropSchema(ctx, root, db, "landing", true); err != nil {
		t.Fatalf("drop cascade: %v", err)
	}
	left := scalar(`SELECT count(*) FROM pg_namespace WHERE nspname = 'landing'`)
	view := scalar(`SELECT count(*) FROM pg_class WHERE relname = 'raw_ids'`)
	if left != "0" || view != "0" {
		t.Errorf("after cascade: schema %s, dependent view %s", left, view)
	}
	if err := pg.DropSchema(ctx, root, db, "information_schema", true); err == nil {
		t.Error("dropped information_schema")
	}
	if _, err := pg.SchemaDependents(ctx, root, db, "nope"); err == nil {
		t.Error("dependents of a missing schema")
	}
}

// TestIntegrationPostgresMoveObject moves a table (with its serial sequence),
// a view, a function and an enum to another schema, and refuses what it
// cannot move.
func TestIntegrationPostgresMoveObject(t *testing.T) {
	ctx := context.Background()
	root := itParams(t, "FLEETDOCK_IT_POSTGRES", "postgres")
	pg := &Postgres{}
	waitReady(t, pg, root)
	const db = "fd_it_move"
	mustQuery(t, pg, root, "postgres", "DROP DATABASE IF EXISTS "+db, true)
	mustQuery(t, pg, root, "postgres", "CREATE DATABASE "+db, true)
	t.Cleanup(func() { mustQuery(t, pg, root, "postgres", "DROP DATABASE IF EXISTS "+db, true) })
	for _, stmt := range []string{
		"CREATE SCHEMA archive",
		"CREATE TYPE mood AS ENUM ('ok', 'bad')",
		"CREATE TABLE orders (id serial PRIMARY KEY, how mood)",
		"CREATE VIEW recent AS SELECT * FROM orders",
		"CREATE FUNCTION total() RETURNS bigint LANGUAGE sql AS 'SELECT count(*) FROM public.orders'",
		"CREATE FUNCTION twice(int) RETURNS int LANGUAGE sql AS 'SELECT $1 * 2'",
		"CREATE FUNCTION twice(bigint) RETURNS bigint LANGUAGE sql AS 'SELECT $1 * 2'",
		"CREATE TABLE archive.recent (id int)",
	} {
		mustQuery(t, pg, root, db, stmt, true)
	}
	// where lists the schemas holding a relation of that name.
	where := func(rel string) string {
		res := mustQuery(t, pg, root, db, `SELECT string_agg(n.nspname, ',' ORDER BY n.nspname) FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relname = '`+rel+`'`, false)
		if len(res.Rows) == 0 || res.Rows[0][0] == nil {
			return ""
		}
		return *res.Rows[0][0]
	}

	for _, obj := range []SchemaObject{
		{Kind: "relation", Schema: "public", Name: "orders"},
		{Kind: "function", Schema: "public", Name: "total"},
		{Kind: "type", Schema: "public", Name: "mood"},
	} {
		if err := pg.MoveObject(ctx, root, db, obj, "archive"); err != nil {
			t.Fatalf("move %+v: %v", obj, err)
		}
	}
	if got := where("orders"); got != "archive" {
		t.Errorf("orders in %q", got)
	}
	if got := where("orders_id_seq"); got != "archive" {
		t.Errorf("the serial's sequence stayed behind: %q", got)
	}
	// The view in public still works: it points at the table, not its name.
	mustQuery(t, pg, root, db, "SELECT * FROM public.recent", false)

	// archive already has a relation called recent.
	if err := pg.MoveObject(ctx, root, db, SchemaObject{Kind: "relation", Schema: "public", Name: "recent"}, "archive"); err == nil ||
		!strings.Contains(err.Error(), "already has") {
		t.Errorf("name clash: %v", err)
	}
	if err := pg.MoveObject(ctx, root, db, SchemaObject{Kind: "function", Schema: "public", Name: "twice"}, "archive"); err == nil ||
		!strings.Contains(err.Error(), "there are 2") {
		t.Errorf("overloaded function: %v", err)
	}
	// With its arguments, one overload moves.
	if err := pg.MoveObject(ctx, root, db, SchemaObject{Kind: "function", Schema: "public", Name: "twice(bigint)"}, "archive"); err != nil {
		t.Errorf("move twice(bigint): %v", err)
	}
	if n := mustQuery(t, pg, root, db, `SELECT count(*) FROM pg_proc WHERE proname = 'twice' AND pronamespace = 'archive'::regnamespace`, false); *n.Rows[0][0] != "1" {
		t.Errorf("overloads in archive = %s", *n.Rows[0][0])
	}
	if err := pg.MoveObject(ctx, root, db, SchemaObject{Kind: "relation", Schema: "public", Name: "nope"}, "archive"); err == nil {
		t.Error("moved a missing table")
	}
	if err := pg.MoveObject(ctx, root, db, SchemaObject{Kind: "relation", Schema: "archive", Name: "orders"}, "pg_catalog"); err == nil {
		t.Error("moved into pg_catalog")
	}
	if err := pg.MoveObject(ctx, root, db, SchemaObject{Kind: "trigger", Schema: "archive", Name: "x"}, "public"); err == nil {
		t.Error("moved a trigger")
	}
}
