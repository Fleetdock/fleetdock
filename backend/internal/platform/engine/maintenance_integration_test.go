//go:build integration

package engine

import (
	"context"
	"strings"
	"testing"
)

// TestIntegrationPostgresMaintenance reads and sets a sequence, refreshes a
// materialized view, vacuums and analyzes, reads table statistics and lists
// partitions, which the table list nests under their parent.
func TestIntegrationPostgresMaintenance(t *testing.T) {
	ctx := context.Background()
	root := itParams(t, "FLEETDOCK_IT_POSTGRES", "postgres")
	pg := &Postgres{}
	waitReady(t, pg, root)
	const db = "fd_it_maint"
	mustQuery(t, pg, root, "postgres", "DROP DATABASE IF EXISTS "+db, true)
	mustQuery(t, pg, root, "postgres", "CREATE DATABASE "+db, true)
	t.Cleanup(func() { mustQuery(t, pg, root, "postgres", "DROP DATABASE IF EXISTS "+db, true) })
	for _, stmt := range []string{
		"CREATE SCHEMA app",
		"CREATE TABLE app.orders (id serial PRIMARY KEY, total int)",
		"INSERT INTO app.orders (total) SELECT g FROM generate_series(1, 50) g",
		"DELETE FROM app.orders WHERE id <= 10",
		"CREATE MATERIALIZED VIEW app.big AS SELECT id FROM app.orders WHERE total > 40",
		"CREATE UNIQUE INDEX ON app.big (id)",
		"INSERT INTO app.orders (total) VALUES (99)",
		"CREATE TABLE app.events (at date NOT NULL, body text) PARTITION BY RANGE (at)",
		"CREATE TABLE app.events_2026_01 PARTITION OF app.events FOR VALUES FROM ('2026-01-01') TO ('2026-02-01')",
		"CREATE TABLE app.events_2026_02 PARTITION OF app.events FOR VALUES FROM ('2026-02-01') TO ('2026-03-01')",
		"INSERT INTO app.events VALUES ('2026-01-05', 'a'), ('2026-02-07', 'b')",
	} {
		mustQuery(t, pg, root, db, stmt, true)
	}

	seq, err := pg.Sequence(ctx, root, db, "app", "orders_id_seq")
	if err != nil {
		t.Fatal(err)
	}
	if seq.LastValue == nil || *seq.LastValue != 51 || seq.NextValue == nil || *seq.NextValue != 52 || seq.OwnedBy != "app.orders.id" || seq.Increment != 1 {
		t.Errorf("sequence = %+v", seq)
	}
	if err := pg.SetSequence(ctx, root, db, "app", "orders_id_seq", 1000, false); err != nil {
		t.Fatalf("setval: %v", err)
	}
	if seq, _ := pg.Sequence(ctx, root, db, "app", "orders_id_seq"); seq.NextValue == nil || *seq.NextValue != 1000 {
		t.Errorf("next value after setval(1000, false) = %v", seq.NextValue)
	}
	if res := mustQuery(t, pg, root, db, "SELECT nextval('app.orders_id_seq')", true); *res.Rows[0][0] != "1000" {
		t.Errorf("nextval after setval(1000, false) = %s", *res.Rows[0][0])
	}
	if err := pg.SetSequence(ctx, root, db, "app", "orders", 1, true); err == nil {
		t.Error("setval on a table")
	}

	mustQuery(t, pg, root, db, "CREATE MATERIALIZED VIEW app.nokey AS SELECT total FROM app.orders", true)
	if err := pg.RefreshMaterializedView(ctx, root, db, "app", "nokey", true); err == nil || !strings.Contains(err.Error(), "unique index") {
		t.Errorf("concurrent refresh without a unique index: %v", err)
	}
	if err := pg.RefreshMaterializedView(ctx, root, db, "app", "big", true); err != nil {
		t.Fatalf("refresh concurrently: %v", err)
	}
	if res := mustQuery(t, pg, root, db, "SELECT count(*) FROM app.big", false); *res.Rows[0][0] != "11" {
		t.Errorf("view rows after refresh = %s", *res.Rows[0][0])
	}

	ran, err := pg.Maintain(ctx, root, db, OpVacuumAnalyze, "app", "orders")
	if err != nil || len(ran) != 1 || !strings.HasPrefix(ran[0], "VACUUM (ANALYZE)") {
		t.Fatalf("vacuum analyze: %v %v", ran, err)
	}
	if ran, err := pg.Maintain(ctx, root, db, OpAnalyze, "app", ""); err != nil || len(ran) < 3 {
		t.Errorf("analyze schema: %v %v", ran, err)
	}
	if ran, err := pg.Maintain(ctx, root, db, OpReindex, "app", ""); err != nil || len(ran) != 1 || ran[0] != `REINDEX SCHEMA "app"` {
		t.Errorf("reindex schema: %v %v", ran, err)
	}
	if _, err := pg.Maintain(ctx, root, db, "drop", "app", ""); err == nil {
		t.Error("unknown op ran")
	}
	if _, err := pg.Maintain(ctx, root, db, OpVacuum, "pg_catalog", ""); err == nil {
		t.Error("vacuumed pg_catalog through a schema run")
	}

	stats, err := pg.TableStats(ctx, root, db, "app")
	if err != nil {
		t.Fatal(err)
	}
	var orders *TableStat
	for i := range stats {
		if stats[i].Name == "orders" {
			orders = &stats[i]
		}
	}
	if orders == nil || orders.LastVacuum == nil || orders.LastAnalyze == nil || orders.LiveRows != 41 || orders.TotalBytes == 0 {
		t.Errorf("orders stats = %+v", orders)
	}

	parts, err := pg.Partitions(ctx, root, db, "app", "events")
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 || parts[0].Name != "events_2026_01" || !strings.Contains(parts[0].Bound, "2026-01-01") {
		t.Errorf("partitions = %+v", parts)
	}
	if _, err := pg.Partitions(ctx, root, db, "app", "orders"); err == nil {
		t.Error("partitions of a plain table")
	}

	tables, err := pg.ListTables(ctx, root, db)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]TableInfo{}
	for _, ti := range tables {
		got[ti.Schema+"."+ti.Name] = ti
	}
	if !got["app.events"].Partitioned || got["app.events_2026_01"].PartitionOf != "app.events" || got["app.orders"].Partitioned {
		t.Errorf("table list: %+v", tables)
	}
}
