//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

func TestIntegrationCatalog(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, scratchDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	instances := NewInstanceRepository(pool)
	dbs := NewDatabaseRepository(pool)
	repo := NewCatalogRepository(pool)

	user := "root"
	inst, err := instancedom.NewExternal("primary", instancedom.EngineMariaDB, "11.4", "db.example.com", 3306, &user, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := instances.Create(ctx, inst); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	if _, err := dbs.Reconcile(ctx, inst.ID, []databasedom.Observed{{Name: "shop"}, {Name: "crm"}, {Name: "mysql"}}, t0); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT name, id::text FROM databases`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n, id string
		_ = rows.Scan(&n, &id)
		ids[n] = id
	}
	rows.Close()

	// Without admin credentials nothing is due.
	due, err := repo.ListDue(ctx, t0, t0, 10)
	if err != nil || len(due) != 0 {
		t.Fatalf("due without credentials = %v, %v", due, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO secrets (ref, kind, ciphertext, encrypted_data_key, key_id, nonce) VALUES ('s1', 'mariadb_root', '\x00', '\x00', 'k', '\x00');
		UPDATE instances SET root_secret_ref = 's1';
		UPDATE databases SET system = true WHERE name = 'mysql'`); err != nil {
		t.Fatal(err)
	}
	due, err = repo.ListDue(ctx, t0, t0, 10)
	if err != nil || len(due) != 2 {
		t.Fatalf("due = %v, %v (system database must be skipped)", due, err)
	}

	shop := due[0]
	if ids["shop"] != shop.String() {
		shop = due[1]
	}
	cat := &engine.ColumnCatalog{Tables: []engine.CatalogTable{
		{Name: "customers", Kind: "table", Columns: []engine.CatalogColumn{{Name: "id", Type: "int"}, {Name: "customer_email", Type: "varchar(200)"}}},
		{Name: "email_log", Kind: "view", Columns: []engine.CatalogColumn{{Name: "sent_at", Type: "datetime"}}},
	}}
	if err := repo.Replace(ctx, shop, cat, t0); err != nil {
		t.Fatal(err)
	}
	// Refreshing twice replaces rather than duplicates.
	if err := repo.Replace(ctx, shop, cat, t0); err != nil {
		t.Fatal(err)
	}
	if err := repo.Failed(ctx, mustUUID(t, ids["crm"]), "connection refused", t0); err != nil {
		t.Fatal(err)
	}

	// Fresh catalogs and recent failures are not due again.
	due, _ = repo.ListDue(ctx, t0.Add(-time.Minute), t0.Add(-time.Minute), 10)
	if len(due) != 0 {
		t.Fatalf("due right after refresh = %v", due)
	}
	due, _ = repo.ListDue(ctx, t0.Add(time.Hour), t0.Add(time.Hour), 10)
	if len(due) != 2 {
		t.Fatalf("due after an hour = %v", due)
	}

	hits, err := repo.Search(ctx, "EMAIL", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %+v", hits)
	}
	// Prefix match on the view's name ranks before the substring column match.
	if hits[0].Table != "email_log" || hits[0].Column != nil || hits[0].Kind != "view" || hits[0].DatabaseName != "shop" || hits[0].InstanceName != "primary" {
		t.Errorf("first hit = %+v", hits[0])
	}
	if hits[1].Column == nil || *hits[1].Column != "customer_email" || hits[1].Type != "varchar(200)" || hits[1].Table != "customers" {
		t.Errorf("second hit = %+v", hits[1])
	}
	// LIKE metacharacters are literal.
	if hits, _ := repo.Search(ctx, "_", 10); len(hits) != 3 {
		t.Errorf("'_' should match only names containing an underscore, got %+v", hits)
	}
	if hits, _ := repo.Search(ctx, "%", 10); len(hits) != 0 {
		t.Errorf("'%%' matched %+v", hits)
	}
}

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
