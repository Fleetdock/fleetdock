//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	tableviewdom "github.com/Fleetdock/fleetdock/backend/internal/domain/tableview"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

func TestIntegrationTableViews(t *testing.T) {
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
	inst, err := instancedom.NewExternal("pg", instancedom.EnginePostgres, "16", "db.example.com", 5432, &user, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewInstanceRepository(pool).Create(ctx, inst); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDatabaseRepository(pool).Reconcile(ctx, inst.ID, []databasedom.Observed{{Name: "shop"}}, inst.CreatedAt); err != nil {
		t.Fatal(err)
	}
	var dbID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM databases WHERE name = 'shop'`).Scan(&dbID); err != nil {
		t.Fatal(err)
	}
	ann, bob := uuid.New(), uuid.New()
	for _, u := range []uuid.UUID{ann, bob} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, 'x', 'x')`, u, u.String()+"@x.dev"); err != nil {
			t.Fatal(err)
		}
	}

	repo := NewTableViewRepository(pool)
	state := json.RawMessage(`{"filters": [{"column": "status", "op": "eq", "value": "paid"}]}`)
	mine := &tableviewdom.View{DatabaseID: dbID, Table: "public.orders", Name: "Paid", State: state, CreatedBy: &ann}
	if err := repo.Create(ctx, mine); err != nil {
		t.Fatal(err)
	}
	dup := &tableviewdom.View{DatabaseID: dbID, Table: "public.orders", Name: "paid", State: state, CreatedBy: &ann}
	if err := repo.Create(ctx, dup); err == nil || err.(*apperr.Error).Kind != apperr.KindConflict {
		t.Errorf("duplicate name = %v", err)
	}
	team := &tableviewdom.View{DatabaseID: dbID, Table: "public.orders", Name: "Team", State: state, CreatedBy: &bob, Shared: true}
	if err := repo.Create(ctx, team); err != nil {
		t.Fatal(err)
	}

	l, err := repo.List(ctx, dbID, "public.orders", ann)
	if err != nil || len(l) != 2 || l[1].AuthorEmail != bob.String()+"@x.dev" {
		t.Fatalf("ann's list = %+v, %v", l, err)
	}
	if l, _ := repo.List(ctx, dbID, "", bob); len(l) != 1 || l[0].ID != team.ID {
		t.Errorf("bob's list = %+v", l)
	}
	if l, _ := repo.List(ctx, dbID, "public.other", ann); len(l) != 0 {
		t.Errorf("other table = %+v", l)
	}

	mine.Name, mine.Shared = "Paid orders", true
	if err := repo.Update(ctx, mine); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, dbID, mine.ID)
	var a, b any
	_ = json.Unmarshal(state, &a)
	_ = json.Unmarshal(got.State, &b)
	if err != nil || got.Name != "Paid orders" || !got.Shared || !reflect.DeepEqual(a, b) {
		t.Errorf("get = %+v (%s), %v", got, got.State, err)
	}

	// The author leaving keeps the view; the database going removes it.
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, bob); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.Get(ctx, dbID, team.ID); err != nil || got.CreatedBy != nil {
		t.Errorf("after author delete = %+v, %v", got, err)
	}
	if err := repo.Delete(ctx, dbID, mine.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, dbID, mine.ID); err == nil {
		t.Error("second delete succeeded")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM databases WHERE id = $1`, dbID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM table_views`).Scan(&n)
	if n != 0 {
		t.Errorf("views left after the database = %d", n)
	}
}
