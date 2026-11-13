//go:build integration

package executor

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// TestIntegrationSchemaBackupRestore backs up one schema and restores it
// into another database under its own name, into the same database under a
// new name, and over itself. Needs FLEETDOCK_IT_PG_TOOLS_POSTGRES.
func TestIntegrationSchemaBackupRestore(t *testing.T) {
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

	var (
		mu       sync.Mutex
		artifact []byte
	)
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPut {
			artifact, _ = io.ReadAll(r.Body)
			return
		}
		_, _ = w.Write(artifact)
	}))
	defer store.Close()

	for _, db := range []string{"fd_it_sb_src", "fd_it_sb_dst"} {
		exec("postgres", "DROP DATABASE IF EXISTS "+db)
		exec("postgres", "CREATE DATABASE "+db)
		t.Cleanup(func() { exec("postgres", "DROP DATABASE IF EXISTS "+db) })
	}
	for _, db := range []string{"fd_it_sb_src", "fd_it_sb_dst"} {
		exec(db, "CREATE SCHEMA ref")
		exec(db, "CREATE TABLE ref.country (code text PRIMARY KEY)")
		exec(db, "INSERT INTO ref.country VALUES ('TJ'), ('DE')")
	}
	for _, stmt := range []string{
		"CREATE SCHEMA sales",
		"CREATE TABLE sales.customers (id serial PRIMARY KEY, country text REFERENCES ref.country)",
		"INSERT INTO sales.customers (country) VALUES ('TJ'), ('DE'), ('TJ')",
		"CREATE VIEW sales.tj AS SELECT * FROM sales.customers WHERE country = 'TJ'",
		"CREATE TABLE public.other (id int)",
	} {
		exec("fd_it_sb_src", stmt)
	}

	raw, err := Execute(ctx, "backup", &Payload{Engine: "postgres", Conn: conn, Database: "fd_it_sb_src", Schema: "sales", PutURL: store.URL}, NopSink{})
	if err != nil {
		t.Fatalf("schema backup: %v", err)
	}
	var res Result
	_ = json.Unmarshal(raw, &res)

	restore := func(db, target string, replace bool) error {
		_, err := Execute(ctx, "restore_schema", &Payload{Engine: "postgres", Conn: conn, Database: db, GetURL: store.URL,
			Checksum: res.Checksum, Schema: "sales", TargetSchema: target, Replace: replace}, NopSink{})
		return err
	}

	// Into another database, under its own name: only the schema arrives.
	if err := restore("fd_it_sb_dst", "sales", false); err != nil {
		t.Fatalf("restore into another database: %v", err)
	}
	if n := scalar("fd_it_sb_dst", "SELECT count(*) FROM sales.tj"); n != "2" {
		t.Errorf("restored view rows = %s", n)
	}
	if n := scalar("fd_it_sb_dst", "SELECT count(*) FROM pg_class WHERE relname = 'other'"); n != "0" {
		t.Error("the schema backup carried another schema's table")
	}
	if err := restore("fd_it_sb_dst", "sales", false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("restore over an existing schema without replace: %v", err)
	}

	// Into the same database under a new name, next to the original.
	exec("fd_it_sb_src", "INSERT INTO sales.customers (country) VALUES ('DE')")
	if err := restore("fd_it_sb_src", "sales_before", false); err != nil {
		t.Fatalf("restore under a new name: %v", err)
	}
	if n := scalar("fd_it_sb_src", "SELECT count(*) FROM sales_before.customers"); n != "3" {
		t.Errorf("rows as backed up = %s", n)
	}
	if n := scalar("fd_it_sb_src", "SELECT count(*) FROM sales.customers"); n != "4" {
		t.Errorf("the original changed: %s rows", n)
	}
	if def := scalar("fd_it_sb_src", "SELECT pg_get_viewdef('sales_before.tj')"); !strings.Contains(def, "sales_before.customers") {
		t.Errorf("restored view reads %s", def)
	}
	if v := scalar("fd_it_sb_src", `SELECT bool_and(convalidated) FROM pg_constraint WHERE conrelid = 'sales_before.customers'::regclass AND contype = 'f'`); v != "t" && v != "true" {
		t.Errorf("foreign key into ref validated = %s", v)
	}

	// Over itself: back to the backed-up rows.
	if err := restore("fd_it_sb_src", "sales", true); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if n := scalar("fd_it_sb_src", "SELECT count(*) FROM sales.customers"); n != "3" {
		t.Errorf("rows after replace = %s", n)
	}
	if n := scalar("postgres", "SELECT count(*) FROM pg_database WHERE datname LIKE 'fds\\_%'"); n != "0" {
		t.Errorf("scratch databases left: %s", n)
	}
}
