//go:build integration

// Repository tests against a real PostgreSQL (FLEETDOCK_IT_POSTGRES, as for the
// engine integration tests). Each run migrates a fresh scratch database.
package postgres

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
)

func scratchDB(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("FLEETDOCK_IT_POSTGRES")
	if addr == "" {
		t.Skip("FLEETDOCK_IT_POSTGRES not set")
	}
	pw := os.Getenv("FLEETDOCK_IT_PASSWORD")
	if pw == "" {
		pw = "rootpw"
	}
	ctx := context.Background()
	admin := fmt.Sprintf("postgres://postgres:%s@%s/postgres?sslmode=disable", pw, addr)
	var conn *pgx.Conn
	var err error
	for i := 0; i < 60; i++ {
		if conn, err = pgx.Connect(ctx, admin); err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	name := fmt.Sprintf("fd_meta_%d", time.Now().UnixNano())
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), admin)
		if err == nil {
			_, _ = c.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			_ = c.Close(context.Background())
		}
	})
	return fmt.Sprintf("postgres://postgres:%s@%s/%s?sslmode=disable", pw, addr, name)
}

func names(xs []string) []string {
	sort.Strings(xs)
	return xs
}

func TestIntegrationReconcile(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, scratchDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Migrations are idempotent.
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	instances := NewInstanceRepository(pool)
	dbs := NewDatabaseRepository(pool)
	user := "root"
	inst, err := instancedom.NewExternal("primary", instancedom.EngineMariaDB, "11.4", "db.example.com", 3306, &user, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := instances.Create(ctx, inst); err != nil {
		t.Fatal(err)
	}

	t0 := time.Now()
	obs := func(ns ...string) []databasedom.Observed {
		out := make([]databasedom.Observed, len(ns))
		for i, n := range ns {
			out[i] = databasedom.Observed{Name: n, Charset: "utf8mb4", Collation: "utf8mb4_bin", SizeBytes: int64(100 * (i + 1)), Connections: i}
		}
		return out
	}
	status := func(name string) databasedom.Status {
		t.Helper()
		page, err := dbs.List(ctx, databasedom.ListFilter{InstanceID: &inst.ID, Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range page.Items {
			if d.Name == name {
				return d.Status
			}
		}
		return "absent"
	}

	// 1. First sight: both added, active, with stats.
	res, err := dbs.Reconcile(ctx, inst.ID, obs("app", "logs"), t0)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(res.Added); len(got) != 2 || got[0] != "app" {
		t.Fatalf("added = %v", got)
	}
	if status("app") != databasedom.StatusActive {
		t.Fatalf("app status = %s", status("app"))
	}

	// 2. "logs" disappears briefly: not yet missing.
	res, _ = dbs.Reconcile(ctx, inst.ID, obs("app"), t0.Add(time.Minute))
	if len(res.Missing) != 0 || status("logs") != databasedom.StatusActive {
		t.Fatalf("logs went missing after one probe: %+v", res)
	}

	// 3. Still gone after MissingAfter: marked missing, not deleted.
	res, _ = dbs.Reconcile(ctx, inst.ID, obs("app"), t0.Add(databasedom.MissingAfter+time.Minute))
	if len(res.Missing) != 1 || res.Missing[0] != "logs" || status("logs") != databasedom.StatusMissing {
		t.Fatalf("missing = %+v, status = %s", res.Missing, status("logs"))
	}

	// 4. It comes back: active again.
	res, _ = dbs.Reconcile(ctx, inst.ID, obs("app", "logs"), t0.Add(10*time.Minute))
	if len(res.Reappeared) != 1 || status("logs") != databasedom.StatusActive {
		t.Fatalf("reappeared = %+v, status = %s", res.Reappeared, status("logs"))
	}

	// 5. A database the user removed from Fleetdock is not re-added right away.
	page, _ := dbs.List(ctx, databasedom.ListFilter{InstanceID: &inst.ID, Limit: 100})
	for _, d := range page.Items {
		if d.Name == "logs" {
			if err := dbs.SoftDelete(ctx, d.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	res, _ = dbs.Reconcile(ctx, inst.ID, obs("app", "logs"), t0.Add(11*time.Minute))
	if len(res.Added) != 0 || status("logs") != "absent" {
		t.Fatalf("removed database re-added: %+v", res)
	}

	// 6. Locked databases are never flipped to missing.
	var appID string
	_ = pool.QueryRow(ctx, `UPDATE databases SET status = 'locked' WHERE instance_id = $1 AND name = 'app' RETURNING id`, inst.ID).Scan(&appID)
	res, _ = dbs.Reconcile(ctx, inst.ID, obs(), t0.Add(30*time.Minute))
	if len(res.Missing) != 0 || status("app") != databasedom.StatusLocked {
		t.Fatalf("locked database changed: %+v, %s", res, status("app"))
	}
}
