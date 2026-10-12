package executor

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// fakePgDump puts a pg_dump stand-in running script on PATH.
func fakePgDump(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pg_dump"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestDumpStreamsGzippedSQL(t *testing.T) {
	fakePgDump(t, `echo "CREATE TABLE t (id int);"`)
	var out bytes.Buffer
	started := false
	err := Dump(context.Background(), "postgres", engine.ConnParams{Host: "db", Port: 5432}, "app", &out, func() { started = true })
	if err != nil {
		t.Fatalf("dump: %v", err)
	}
	if !started {
		t.Fatal("onStart was not called")
	}
	zr, err := gzip.NewReader(&out)
	if err != nil {
		t.Fatalf("output is not gzip: %v", err)
	}
	sql, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sql), "CREATE TABLE t") {
		t.Fatalf("unexpected dump: %q", sql)
	}
}

func TestDumpFailureBeforeOutputReportsStderr(t *testing.T) {
	fakePgDump(t, `echo 'pg_dump: error: connection failed: password authentication failed' >&2; exit 1`)
	var out bytes.Buffer
	started := false
	err := Dump(context.Background(), "postgres", engine.ConnParams{Host: "db", Port: 5432}, "app", &out, func() { started = true })
	if err == nil || !strings.Contains(err.Error(), "password authentication failed") {
		t.Fatalf("expected the tool's error, got %v", err)
	}
	if started || out.Len() != 0 {
		t.Fatal("nothing may be written when the dump fails to start")
	}
}

func TestDumpPreflightMissingTool(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := DumpPreflight("postgres"); err == nil {
		t.Fatal("expected an error when pg_dump is not installed")
	}
	fakePgDump(t, "true")
	if err := DumpPreflight("postgres"); err != nil {
		t.Fatalf("preflight: %v", err)
	}
}
