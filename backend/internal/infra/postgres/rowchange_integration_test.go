//go:build integration

package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	rowchangedom "github.com/Fleetdock/fleetdock/backend/internal/domain/rowchange"
)

func TestIntegrationRowChanges(t *testing.T) {
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
	inst, err := instancedom.NewExternal("primary", instancedom.EngineMariaDB, "11.4", "db.example.com", 3306, &user, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewInstanceRepository(pool).Create(ctx, inst); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDatabaseRepository(pool).Reconcile(ctx, inst.ID, []databasedom.Observed{{Name: "shop"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	var dbID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM databases`).Scan(&dbID); err != nil {
		t.Fatal(err)
	}
	repo := NewRowChangeRepository(pool)
	var users [2]uuid.UUID
	for i := range users {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, name) VALUES ($1, 'u') RETURNING id`,
			uuid.NewString()+"@example.com").Scan(&users[i]); err != nil {
			t.Fatal(err)
		}
	}

	bin := "\x00\xff"
	one := "1"
	batch := uuid.New()
	var ids []uuid.UUID
	for i := 0; i < 3; i++ {
		c := &rowchangedom.Change{
			DatabaseID: dbID, Table: "items", Op: "delete", Source: rowchangedom.SourceEditor,
			Key: rowchangedom.Image{"id": &one}, Before: rowchangedom.Image{"id": &one, "blob": &bin, "note": nil},
			Columns: []string{"id", "blob", "note"}, Session: map[string]string{"time_zone": "+00:00"}, Undoable: true,
		}
		if i < 2 {
			c.BatchID = &batch
		}
		if err := repo.Add(ctx, c); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
		time.Sleep(2 * time.Millisecond)
	}

	got, err := repo.Get(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if *got.Before["blob"] != bin || got.Before["note"] != nil || got.After != nil || got.Session["time_zone"] != "+00:00" {
		t.Fatalf("round trip = %+v", got)
	}

	all, _ := repo.List(ctx, rowchangedom.ListFilter{DatabaseID: dbID, Table: "items"})
	if len(all) != 3 || all[0].ID != ids[2] {
		t.Fatalf("list = %d, newest first?", len(all))
	}
	page, _ := repo.List(ctx, rowchangedom.ListFilter{DatabaseID: dbID, Before: &all[0].CreatedAt})
	if len(page) != 2 {
		t.Fatalf("cursor page = %d", len(page))
	}
	inBatch, _ := repo.List(ctx, rowchangedom.ListFilter{DatabaseID: dbID, BatchID: &batch})
	if len(inBatch) != 2 {
		t.Fatalf("batch = %d", len(inBatch))
	}

	// Two concurrent undos of the same change: exactly one wins.
	var wg sync.WaitGroup
	wins := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.ClaimUndo(ctx, ids[0], users[i], time.Now())
			if err != nil {
				t.Error(err)
			}
			wins <- ok
		}()
	}
	wg.Wait()
	close(wins)
	n := 0
	for ok := range wins {
		if ok {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d claims won", n)
	}
	if err := repo.ReleaseUndo(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	if ok, _ := repo.ClaimUndo(ctx, ids[0], users[1], time.Now()); !ok {
		t.Fatal("a released claim must be claimable again")
	}

	pruned, err := repo.PruneBefore(ctx, time.Now().Add(time.Minute))
	if err != nil || pruned != 3 {
		t.Fatalf("pruned %d, %v", pruned, err)
	}
}
