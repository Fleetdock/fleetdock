//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"

	backupdom "github.com/Fleetdock/fleetdock/backend/internal/domain/backup"
	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
)

// Reads of operations, backups and schedules carry their resources' names.
func TestIntegrationDisplayNames(t *testing.T) {
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
	srv, inst, db, other, bk := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO servers (id, name, hostname) VALUES ($1, 'web-1', 'web-1')`, srv)
	exec(`INSERT INTO instances (id, server_id, name, mariadb_version, port, engine) VALUES ($1, $2, 'main', '16', 5432, 'postgres')`, inst, srv)
	exec(`INSERT INTO databases (id, instance_id, name, status) VALUES ($1, $2, 'orders', 'active'), ($3, $2, 'cache_100%', 'active')`, db, inst, other)
	exec(`INSERT INTO backups (id, database_id, type, engine, status) VALUES ($1, $2, 'manual', 'pg_dump', 'completed'), ($4, $3, 'manual', 'pg_dump', 'completed')`, bk, db, other, uuid.New())
	exec(`INSERT INTO backup_schedules (database_id, cron, engine) VALUES ($1, '0 3 * * *', 'mariadb-dump')`, db)

	jobs := NewJobRepository(pool)
	for rt, id := range map[string]uuid.UUID{"database": db, "instance": inst, "server": srv, "backup": bk} {
		id := id
		j := &jobdom.Job{ID: uuid.New(), Type: "backup", ResourceType: rt, ResourceID: &id, Status: "pending"}
		if err := jobs.Create(ctx, j); err != nil {
			t.Fatal(err)
		}
		got, err := jobs.GetByID(ctx, j.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"database": "orders", "instance": "main", "server": "web-1", "backup": "orders"}[rt]
		if got.ResourceName != want {
			t.Errorf("%s job: resource name %q, want %q", rt, got.ResourceName, want)
		}
	}
	page, err := jobs.List(ctx, jobdom.ListFilter{Limit: 10})
	if err != nil || len(page.Items) != 4 || page.Items[0].ResourceName == "" {
		t.Errorf("list jobs = %+v, %v", page.Items, err)
	}
	// Claiming (UPDATE … RETURNING) still works without the name column.
	if _, err := jobs.ClaimNext(ctx, nil); err != nil {
		t.Errorf("claim: %v", err)
	}

	backups := NewBackupRepository(pool)
	b, err := backups.GetByID(ctx, bk)
	if err != nil || b.DatabaseName != "orders" || b.InstanceName != "main" {
		t.Errorf("backup = %+v, %v", b, err)
	}
	for search, want := range map[string]int{"orders": 1, "MAIN": 2, "100%": 1, "%": 1, "e_1": 1, "s_r": 0, "nope": 0} {
		res, err := backups.List(ctx, backupdom.ListFilter{Search: search, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Items) != want {
			t.Errorf("search %q: %d backups, want %d", search, len(res.Items), want)
		}
	}

	scheds, err := NewScheduleRepository(pool).List(ctx, nil)
	if err != nil || len(scheds) != 1 || scheds[0].DatabaseName != "orders" || scheds[0].InstanceName != "main" {
		t.Errorf("schedules = %+v, %v", scheds, err)
	}
}
