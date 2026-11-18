//go:build integration

package executor

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// TestIntegrationRestoreRefusesClientCommands feeds psql a SQL stream holding
// a \! shell escape and checks it is refused, then restores a real pg_dump
// (with COPY data and pg_dump's own \restrict lines) to check that ordinary
// dumps still load. Needs FLEETDOCK_IT_PG_TOOLS_POSTGRES (a server matching
// the local pg_dump and psql).
func TestIntegrationRestoreRefusesClientCommands(t *testing.T) {
	addr := os.Getenv("FLEETDOCK_IT_PG_TOOLS_POSTGRES")
	if addr == "" {
		t.Skip("FLEETDOCK_IT_PG_TOOLS_POSTGRES not set")
	}
	host, ps, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(ps)
	conn := engine.ConnParams{Host: host, Port: port, User: "postgres", Password: os.Getenv("FLEETDOCK_IT_PASSWORD"), TLSMode: engine.TLSDisable}
	ctx := context.Background()
	pg := &engine.Postgres{}
	run := func(db, sql string) {
		t.Helper()
		if _, err := pg.Query(ctx, conn, db, sql, 100, true); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	for _, db := range []string{"fd_restrict_src", "fd_restrict_dst"} {
		run("postgres", "DROP DATABASE IF EXISTS "+db)
		run("postgres", "CREATE DATABASE "+db)
	}

	marker := filepath.Join(t.TempDir(), "pwned")
	evil := "CREATE TABLE ok (a int);\n\\! touch " + marker + "\n"
	err := loadSQL(ctx, pg, conn, "fd_restrict_dst", strings.NewReader(evil), nil, NopSink{})
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("psql ran the \\! command from the stream")
	}
	if err == nil {
		t.Fatal("a stream with a client command restored without error")
	}

	run("fd_restrict_src", `CREATE TABLE notes (body text)`)
	run("fd_restrict_src", `INSERT INTO notes VALUES ('SET transaction_timeout = 0;'), ('plain')`)
	bins, args, env := pg.DumpArgs(conn, "fd_restrict_src")
	bin, err := lookPath(bins)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), env...)
	dump, err := cmd.Output()
	if err != nil {
		t.Fatalf("pg_dump: %v", err)
	}
	if err := loadSQL(ctx, pg, conn, "fd_restrict_dst", strings.NewReader(string(dump)), nil, NopSink{}); err != nil {
		t.Fatalf("restore of a real dump: %v", err)
	}
	res, err := pg.Query(ctx, conn, "fd_restrict_dst", "SELECT count(*) FROM notes", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Rows[0][0]; got == nil || *got != "2" {
		t.Errorf("restored %v rows, want 2 (a COPY row was dropped)", got)
	}
}
