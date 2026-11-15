//go:build integration

package executor

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// TestIntegrationCopySchema copies a schema whose tables reference another
// schema, into the same database and into another one, and checks that the
// copy's own references point at the copy. Needs
// FLEETDOCK_IT_PG_TOOLS_POSTGRES (a server matching the local pg_dump).
func TestIntegrationCopySchema(t *testing.T) {
	addr := os.Getenv("FLEETDOCK_IT_PG_TOOLS_POSTGRES")
	if addr == "" {
		t.Skip("FLEETDOCK_IT_PG_TOOLS_POSTGRES not set")
	}
	host, ps, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(ps)
	conn := engine.ConnParams{Host: host, Port: port, User: "postgres", Password: os.Getenv("FLEETDOCK_IT_PASSWORD"), TLSMode: engine.TLSDisable}
	ctx := context.Background()
	pg := &engine.Postgres{}
	exec := func(db, sql string) {
		t.Helper()
		if _, err := pg.Query(ctx, conn, db, sql, 100, true); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	scalar := func(db, sql string) string {
		t.Helper()
		res, err := pg.Query(ctx, conn, db, sql, 100, false)
		if err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		if len(res.Rows) == 0 || res.Rows[0][0] == nil {
			return ""
		}
		return *res.Rows[0][0]
	}
	for _, db := range []string{"fd_it_copy_src", "fd_it_copy_dst"} {
		exec("postgres", "DROP DATABASE IF EXISTS "+db)
		exec("postgres", "CREATE DATABASE "+db)
		t.Cleanup(func() { exec("postgres", "DROP DATABASE IF EXISTS "+db) })
	}
	for _, stmt := range []string{
		"CREATE SCHEMA ref",
		"CREATE TABLE ref.country (code text PRIMARY KEY)",
		"INSERT INTO ref.country VALUES ('TJ'), ('DE')",
		"CREATE SCHEMA sales",
		"CREATE TABLE sales.customers (id serial PRIMARY KEY, country text REFERENCES ref.country)",
		"CREATE TABLE sales.orders (id serial PRIMARY KEY, customer int REFERENCES sales.customers)",
		"INSERT INTO sales.customers (country) VALUES ('TJ'), ('DE')",
		"INSERT INTO sales.orders (customer) VALUES (1), (2), (2)",
		"CREATE VIEW sales.big AS SELECT customer, count(*) FROM sales.orders GROUP BY customer",
	} {
		exec("fd_it_copy_src", stmt)
	}

	run := func(target, schema string, structureOnly bool) error {
		_, err := runCopySchema(ctx, &Payload{Engine: "postgres", Conn: conn, Database: "fd_it_copy_src",
			Schema: "sales", TargetSchema: schema, TargetDatabase: target, StructureOnly: structureOnly}, NopSink{})
		return err
	}

	if err := run("fd_it_copy_src", "sales_copy", false); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if n := scalar("fd_it_copy_src", "SELECT count(*) FROM sales_copy.orders"); n != "3" {
		t.Errorf("orders copied = %s", n)
	}
	// The copy's view and foreign key point at the copy, not the source.
	if def := scalar("fd_it_copy_src", "SELECT pg_get_viewdef('sales_copy.big'::regclass)"); !strings.Contains(def, "sales_copy.orders") {
		t.Errorf("view still reads the source: %s", def)
	}
	if fk := scalar("fd_it_copy_src", `SELECT confrelid::regclass::text FROM pg_constraint WHERE conrelid = 'sales_copy.orders'::regclass AND contype = 'f'`); fk != "sales_copy.customers" {
		t.Errorf("orders FK points at %s", fk)
	}
	// The foreign key into ref, loaded NOT VALID on the way, is checked again.
	if v := scalar("fd_it_copy_src", `SELECT bool_and(convalidated) FROM pg_constraint WHERE conrelid = 'sales_copy.customers'::regclass AND contype = 'f'`); v != "t" && v != "true" {
		t.Errorf("customers FK validated = %s", v)
	}
	// The serial goes on from the copied rows.
	exec("fd_it_copy_src", "INSERT INTO sales_copy.orders (customer) VALUES (1)")
	if id := scalar("fd_it_copy_src", "SELECT max(id) FROM sales_copy.orders"); id != "4" {
		t.Errorf("next id = %s", id)
	}
	// The source is untouched, and no scratch database is left behind.
	if n := scalar("fd_it_copy_src", "SELECT count(*) FROM sales.orders"); n != "3" {
		t.Errorf("source orders = %s", n)
	}
	if n := scalar("postgres", "SELECT count(*) FROM pg_database WHERE datname LIKE 'fds\\_%'"); n != "0" {
		t.Errorf("scratch databases left: %s", n)
	}

	// Into another database, structure only: the referenced ref schema is not
	// there, so the copy fails cleanly and leaves nothing behind.
	if err := run("fd_it_copy_dst", "sales", true); err == nil || !strings.Contains(err.Error(), "ref") {
		t.Errorf("copy without the referenced schema: %v", err)
	}
	if n := scalar("fd_it_copy_dst", "SELECT count(*) FROM pg_namespace WHERE nspname = 'sales'"); n != "0" {
		t.Errorf("a failed copy left schema sales behind")
	}
	exec("fd_it_copy_dst", "CREATE SCHEMA ref")
	exec("fd_it_copy_dst", "CREATE TABLE ref.country (code text PRIMARY KEY)")
	if err := run("fd_it_copy_dst", "sales", true); err != nil {
		t.Fatalf("structure-only copy: %v", err)
	}
	if n := scalar("fd_it_copy_dst", "SELECT count(*) FROM sales.orders"); n != "0" {
		t.Errorf("structure-only copy has %s orders", n)
	}

	if err := run("fd_it_copy_src", "sales_copy", false); err == nil {
		t.Error("copied over an existing schema")
	}
}
