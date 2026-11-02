package postgres

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migration is one embedded SQL file.
type migration struct {
	name     string
	body     []byte
	checksum string
}

func embeddedMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	out := make([]migration, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", e.Name(), err)
		}
		sum := sha256.Sum256(body)
		out = append(out, migration{name: e.Name(), body: body, checksum: hex.EncodeToString(sum[:])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

// ensureMigrationTable creates the bookkeeping table, and adds the checksum
// column to one created before 1.0.
func ensureMigrationTable(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now(),
			checksum   text
		)`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum text`); err != nil {
		return fmt.Errorf("ensure schema_migrations.checksum: %w", err)
	}
	return nil
}

// loadApplied returns the recorded migrations: name -> checksum ("" when
// recorded before checksums were kept).
func loadApplied(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx, `SELECT version, COALESCE(checksum, '') FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, sum string
		if err := rows.Scan(&name, &sum); err != nil {
			return nil, fmt.Errorf("read schema_migrations: %w", err)
		}
		out[name] = sum
	}
	return out, rows.Err()
}

// checkHistory compares what the database has applied with what this build
// embeds. It refuses a database migrated by a newer Fleetdock (an older
// binary would run against a schema it does not know) and a migration whose
// file changed after it was applied (the schema no longer matches the code).
func checkHistory(files []migration, applied map[string]string) error {
	known := make(map[string]string, len(files))
	for _, m := range files {
		known[m.name] = m.checksum
	}
	var unknown, changed []string
	for name, sum := range applied {
		want, ok := known[name]
		switch {
		case !ok:
			unknown = append(unknown, name)
		case sum != "" && sum != want:
			changed = append(changed, name)
		}
	}
	sort.Strings(unknown)
	sort.Strings(changed)
	if len(unknown) > 0 {
		return fmt.Errorf("the metadata database was migrated by a newer Fleetdock (%s not known to this version): "+
			"run that version or newer, or restore the backup taken before the upgrade (see docs/OPERATIONS.md, Rollback)",
			strings.Join(unknown, ", "))
	}
	if len(changed) > 0 {
		return fmt.Errorf("migration %s changed after it was applied: the schema may not match this build; "+
			"never edit a released migration, add a new one", strings.Join(changed, ", "))
	}
	return nil
}

// Migrate applies embedded SQL migrations that have not yet been recorded, in
// filename order. Each migration runs inside its own transaction together with
// the bookkeeping insert, so a failure leaves the schema untouched. The
// embedded .sql files must NOT contain their own BEGIN/COMMIT.
//
// Before applying anything it checks the recorded history (see checkHistory),
// and records each file's SHA-256; migrations recorded before checksums were
// kept get theirs on the first run of this version.
//
// Replicas starting together serialise on an advisory lock, so each migration
// is applied exactly once.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	release, err := lock(ctx, pool, lockKeyMigrations)
	if err != nil {
		return err
	}
	defer release()

	if err := ensureMigrationTable(ctx, pool); err != nil {
		return err
	}
	files, err := embeddedMigrations()
	if err != nil {
		return err
	}
	applied, err := loadApplied(ctx, pool)
	if err != nil {
		return err
	}
	if err := checkHistory(files, applied); err != nil {
		return err
	}

	for _, m := range files {
		if sum, ok := applied[m.name]; ok {
			if sum == "" {
				if _, err := pool.Exec(ctx, `UPDATE schema_migrations SET checksum = $2 WHERE version = $1 AND checksum IS NULL`,
					m.name, m.checksum); err != nil {
					return fmt.Errorf("record checksum of %s: %w", m.name, err)
				}
			}
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", m.name, err)
		}
		if _, err := tx.Exec(ctx, string(m.body)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", m.name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)`, m.name, m.checksum); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", m.name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", m.name, err)
		}
	}
	return nil
}

// CheckSchema verifies, without changing anything, that the metadata
// database is exactly at this build's schema. It is for processes started
// with FLEETDOCK_RUN_MIGRATIONS=false, where another step migrates: they must
// not serve against a schema that is behind or ahead of them.
func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return fmt.Errorf("check schema_migrations: %w", err)
	}
	if !exists {
		return fmt.Errorf("the metadata database has no schema yet: run the migrations (cmd/migrate) or start with FLEETDOCK_RUN_MIGRATIONS=true")
	}
	files, err := embeddedMigrations()
	if err != nil {
		return err
	}
	applied, err := loadApplied(ctx, pool)
	if err != nil {
		// A pre-1.0 table without the checksum column.
		return fmt.Errorf("%w (run this version's migrations first)", err)
	}
	if err := checkHistory(files, applied); err != nil {
		return err
	}
	var pending []string
	for _, m := range files {
		if _, ok := applied[m.name]; !ok {
			pending = append(pending, m.name)
		}
	}
	if len(pending) > 0 {
		return fmt.Errorf("the metadata database is behind this version (%d migration(s) pending, from %s): "+
			"run the migrations first (cmd/migrate) or start with FLEETDOCK_RUN_MIGRATIONS=true", len(pending), pending[0])
	}
	return nil
}
