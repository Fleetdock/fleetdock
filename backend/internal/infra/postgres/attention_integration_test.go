//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"

	statsdom "github.com/Fleetdock/fleetdock/backend/internal/domain/stats"
	"github.com/Fleetdock/fleetdock/backend/internal/version"
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

func TestIntegrationAttentionAgentVersions(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, scratchDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	old := version.Version
	t.Cleanup(func() { version.Version = old })

	server := func(name, status, agent string) uuid.UUID {
		id := uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO servers (id, name, hostname, status, agent_version, last_heartbeat_at)
		      VALUES ($1, $2, $2, $3, NULLIF($4, ''), now())`, id, name, status, agent); err != nil {
			t.Fatal(err)
		}
		return id
	}
	outdated := server("old", "online", "0.9.0")
	vprefixed := server("old-v", "online", "v1.1.9-rc.1")
	unsupported := server("ancient", "online", "0.0.1")
	server("current", "online", "1.2.0")
	server("newer", "online", "1.3.0")
	server("dev", "online", "dev")
	server("none", "online", "")
	server("down", "offline", "0.9.0") // reported as offline instead

	attention := func() map[uuid.UUID]statsdom.Attention {
		items, err := NewStatsRepository(pool).Attention(ctx, 100)
		if err != nil {
			t.Fatal(err)
		}
		out := map[uuid.UUID]statsdom.Attention{}
		for _, a := range items {
			if a.Kind == statsdom.KindAgentOutdated || a.Kind == statsdom.KindAgentUnsupported {
				out[a.ResourceID] = a
			}
		}
		return out
	}

	version.Version = "dev"
	if got := attention(); len(got) != 0 {
		t.Fatalf("a development build must not judge agents: %+v", got)
	}

	version.Version = "v1.2.0"
	got := attention()
	if len(got) != 3 {
		t.Fatalf("got %d agent items, want 3: %+v", len(got), got)
	}
	for id, kind := range map[uuid.UUID]string{
		outdated:    statsdom.KindAgentOutdated,
		vprefixed:   statsdom.KindAgentOutdated,
		unsupported: statsdom.KindAgentUnsupported,
	} {
		if got[id].Kind != kind {
			t.Errorf("server %s: %+v, want %s", id, got[id], kind)
		}
	}
	if got[unsupported].Severity != statsdom.SeverityCritical || got[outdated].Severity != statsdom.SeverityWarning {
		t.Errorf("severities: %+v", got)
	}
}
