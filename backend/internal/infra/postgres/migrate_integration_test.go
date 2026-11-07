//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"
)

func TestIntegrationMigrateHistoryChecks(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, scratchDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	if err := CheckSchema(ctx, pool); err == nil || !strings.Contains(err.Error(), "no schema yet") {
		t.Fatalf("empty database: %v", err)
	}

	// A database migrated before 0.10: the bookkeeping table without the
	// checksum column, and all but the last migration applied.
	files, err := embeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	exec(`CREATE TABLE schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`)
	for _, m := range files[:len(files)-1] {
		exec(string(m.body))
		exec(`INSERT INTO schema_migrations (version) VALUES ($1)`, m.name)
	}
	if err := CheckSchema(ctx, pool); err == nil {
		t.Fatal("CheckSchema accepted a pre-0.10 table")
	}

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	var missing int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE checksum IS NULL OR length(checksum) <> 64`).Scan(&missing); err != nil {
		t.Fatal(err)
	}
	if missing != 0 {
		t.Fatalf("%d migrations without a checksum after the upgrade", missing)
	}
	if err := CheckSchema(ctx, pool); err != nil {
		t.Fatalf("CheckSchema after Migrate: %v", err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second run: %v", err)
	}

	// An edited migration is refused.
	exec(`UPDATE schema_migrations SET checksum = repeat('0', 64) WHERE version = $1`, files[0].name)
	if err := Migrate(ctx, pool); err == nil || !strings.Contains(err.Error(), "changed after it was applied") {
		t.Fatalf("edited migration: %v", err)
	}
	exec(`UPDATE schema_migrations SET checksum = $2 WHERE version = $1`, files[0].name, files[0].checksum)

	// A database a newer Fleetdock migrated is refused, by both paths.
	exec(`INSERT INTO schema_migrations (version, checksum) VALUES ('9999_from_the_future.sql', 'x')`)
	for name, f := range map[string]func(context.Context) error{
		"Migrate":     func(ctx context.Context) error { return Migrate(ctx, pool) },
		"CheckSchema": func(ctx context.Context) error { return CheckSchema(ctx, pool) },
	} {
		if err := f(ctx); err == nil || !strings.Contains(err.Error(), "newer Fleetdock") {
			t.Errorf("%s with a newer schema: %v", name, err)
		}
	}
	exec(`DELETE FROM schema_migrations WHERE version = '9999_from_the_future.sql'`)

	// A pending migration: CheckSchema says so.
	exec(`DELETE FROM schema_migrations WHERE version = $1`, files[len(files)-1].name)
	if err := CheckSchema(ctx, pool); err == nil || !strings.Contains(err.Error(), "behind this version") {
		t.Errorf("pending migration: %v", err)
	}
}
