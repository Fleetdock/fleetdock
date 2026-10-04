//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"

	statsdom "github.com/Fleetdock/fleetdock/backend/internal/domain/stats"
)

func TestIntegrationAttention(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, scratchDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	srv, inst := uuid.New(), uuid.New()
	exec(`INSERT INTO servers (id, name, hostname, status) VALUES ($1, 'web-1', 'web-1', 'offline')`, srv)
	exec(`INSERT INTO instances (id, server_id, name, mariadb_version, port, engine, kind, health)
	      VALUES ($1, $2, 'main', '16', 5432, 'postgres', 'managed',
	              '{"status":"unreachable","error":"connection refused","checked_at":"2026-01-01T00:00:00Z","latency_ms":0}')`, inst, srv)

	mkdb := func(name, status string) uuid.UUID {
		id := uuid.New()
		exec(`INSERT INTO databases (id, instance_id, name, status, created_at, missing_since)
		      VALUES ($1, $2, $3, $4, now() - interval '30 days', CASE WHEN $4 = 'missing' THEN now() END)`, id, inst, name, status)
		return id
	}
	failed := mkdb("orders", "active")   // latest backup failed
	recovered := mkdb("users", "active") // failed, then succeeded: no item
	stale := mkdb("logs", "active")      // scheduled, no backup in 7 days
	unscheduled := mkdb("tmp", "active") // no schedule, no backup: no item
	missing := mkdb("legacy", "missing") // gone from the server
	checked := mkdb("billing", "active") // backup ok, test restore failed
	_ = unscheduled

	backup := func(db uuid.UUID, status, ago string) uuid.UUID {
		id := uuid.New()
		exec(`INSERT INTO backups (id, database_id, type, engine, status, error, created_at, completed_at)
		      VALUES ($1, $2, 'manual', 'pg_dump', $3, CASE WHEN $3 = 'failed' THEN 'disk full' END,
		              now() - $4::interval, CASE WHEN $3 = 'completed' THEN now() - $4::interval END)`, id, db, status, ago)
		return id
	}
	failedBackup := backup(failed, "failed", "1 hour")
	backup(recovered, "failed", "2 hours")
	backup(recovered, "completed", "1 hour")
	checkedBackup := backup(checked, "completed", "1 hour")
	exec(`UPDATE backups SET verify_status = 'failed', verify_error = 'restore failed', verified_at = now() WHERE id = $1`, checkedBackup)
	exec(`INSERT INTO backup_schedules (database_id, cron, engine) VALUES ($1, '0 3 * * *', 'mydumper')`, stale)

	// A failed lock resolved by a later success yields nothing; a failed
	// unlock does.
	exec(`INSERT INTO jobs (type, resource_type, resource_id, status, error, created_at)
	      VALUES ('lock_database', 'database', $1, 'failed', 'timeout', now() - interval '2 hours'),
	             ('lock_database', 'database', $1, 'succeeded', NULL, now() - interval '1 hour'),
	             ('unlock_database', 'database', $2, 'failed', 'denied', now() - interval '1 hour')`, recovered, stale)

	items, err := NewStatsRepository(pool).Attention(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]statsdom.Attention{}
	for _, a := range items {
		if _, dup := got[a.Kind]; dup {
			t.Errorf("duplicate %s item: %+v", a.Kind, a)
		}
		got[a.Kind] = a
	}
	want := map[string]uuid.UUID{
		statsdom.KindServerOffline:       srv,
		statsdom.KindInstanceUnreachable: inst,
		statsdom.KindBackupFailed:        failedBackup,
		statsdom.KindBackupCheckFailed:   checkedBackup,
		statsdom.KindNoRecentBackup:      stale,
		statsdom.KindDatabaseMissing:     missing,
	}
	for kind, id := range want {
		a, ok := got[kind]
		if !ok {
			t.Errorf("missing %s item", kind)
			continue
		}
		if a.ResourceID != id {
			t.Errorf("%s: resource %s, want %s", kind, a.ResourceID, id)
		}
		if a.ServerID != srv {
			t.Errorf("%s: server %s, want %s", kind, a.ServerID, srv)
		}
	}
	op, ok := got[statsdom.KindOperationFailed]
	if !ok || op.Name != "unlock_database" || op.DatabaseID != stale || op.ServerID != srv {
		t.Errorf("operation item = %+v", op)
	}
	if len(items) != len(want)+1 {
		t.Errorf("got %d items, want %d: %+v", len(items), len(want)+1, items)
	}
	// Critical items come first.
	for i, a := range items {
		if a.Severity == statsdom.SeverityCritical && i > 0 && items[i-1].Severity != statsdom.SeverityCritical {
			t.Errorf("critical item after a warning at %d", i)
		}
	}

	s, err := NewStatsRepository(pool).Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.ServersTotal != 1 || s.InstancesTotal != 1 || s.SchedulesEnabled != 1 || s.DestinationsTotal != 0 {
		t.Errorf("summary = %+v", s)
	}
}
