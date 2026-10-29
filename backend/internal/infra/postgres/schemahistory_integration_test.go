//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	schemahistorydom "github.com/Fleetdock/fleetdock/backend/internal/domain/schemahistory"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/schemadiff"
)

func TestIntegrationSchemaHistory(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, scratchDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := "root"
	inst, _ := instancedom.NewExternal("primary", instancedom.EngineMariaDB, "11.4", "db.example.com", 3306, &user, nil, nil)
	if err := NewInstanceRepository(pool).Create(ctx, inst); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDatabaseRepository(pool).Reconcile(ctx, inst.ID, []databasedom.Observed{{Name: "shop"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	var dbID uuid.UUID
	_ = pool.QueryRow(ctx, `SELECT id FROM databases`).Scan(&dbID)

	repo := NewSchemaHistoryRepository(pool)
	if w, err := repo.GetWatch(ctx, dbID); err != nil || w != nil {
		t.Fatalf("watch before = %+v, %v", w, err)
	}
	if _, err := repo.SetWatch(ctx, dbID, true, nil); err != nil {
		t.Fatal(err)
	}
	due, err := repo.ListDue(ctx, time.Now(), 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("due = %v, %v", due, err)
	}
	if err := repo.MarkChecked(ctx, dbID, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	if due, _ := repo.ListDue(ctx, time.Now().Add(-time.Minute), 10); len(due) != 0 {
		t.Fatalf("checked watch still due: %v", due)
	}

	s := &schemadiff.Schema{Dialect: schemadiff.MySQL, Tables: []schemadiff.Table{{Name: "t", Columns: []schemadiff.Column{{Name: "a", Def: "`a` int"}}}}}
	gz, err := schemahistorydom.Encode(s)
	if err != nil {
		t.Fatal(err)
	}
	var last uuid.UUID
	for i := 0; i < schemahistorydom.Keep+3; i++ {
		snap := &schemahistorydom.Snapshot{DatabaseID: dbID, Hash: s.Hash(), SchemaGz: gz, Trigger: schemahistorydom.TriggerScheduled,
			Summary: schemahistorydom.Summary{Stored: true, Counts: s.Counts()}}
		if i > 0 {
			snap.PreviousID = &last
		}
		if err := repo.Add(ctx, snap); err != nil {
			t.Fatal(err)
		}
		last = snap.ID
	}
	items, err := repo.List(ctx, dbID, 1000)
	if err != nil || len(items) != schemahistorydom.Keep || items[0].ID != last || items[0].SchemaGz != nil {
		t.Fatalf("list = %d items, %v", len(items), err)
	}
	latest, err := repo.Latest(ctx, dbID)
	if err != nil || latest.ID != last || latest.Summary.Counts.Tables != 1 {
		t.Fatalf("latest = %+v, %v", latest, err)
	}
	back, err := schemahistorydom.Decode(latest.SchemaGz)
	if err != nil || back.Hash() != s.Hash() {
		t.Fatalf("decode: %v", err)
	}
	if _, err := repo.Get(ctx, uuid.New(), last); err == nil {
		t.Fatal("got a snapshot through another database")
	}
}
