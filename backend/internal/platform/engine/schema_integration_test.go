//go:build integration

package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/schemadiff"
)

// TestIntegrationSchemaRoundTrip reads two differing databases, writes the
// script that makes the second like the first, runs it, and checks the
// second then hashes like the first (but for the one table only it has,
// whose drop is left for review).
func TestIntegrationSchemaRoundTrip(t *testing.T) {
	mysqlSource := []string{
		"CREATE TABLE orders (id int NOT NULL AUTO_INCREMENT PRIMARY KEY, customer varchar(50) NOT NULL DEFAULT 'x', note text, " +
			"created timestamp NULL DEFAULT CURRENT_TIMESTAMP, KEY ix_customer (customer))",
		"CREATE TABLE items (id int NOT NULL AUTO_INCREMENT PRIMARY KEY, order_id int NOT NULL, qty int NOT NULL DEFAULT 1, " +
			"CONSTRAINT fk_items_order FOREIGN KEY (order_id) REFERENCES orders (id) ON DELETE CASCADE, CONSTRAINT chk_qty CHECK (qty > 0))",
		"CREATE TABLE tags (id int PRIMARY KEY, name varchar(20), UNIQUE KEY u_name (name))",
		"CREATE VIEW v_orders AS SELECT id, customer FROM orders",
		"CREATE PROCEDURE p_count() BEGIN SELECT COUNT(*) FROM orders; END",
		"CREATE TRIGGER trg_items BEFORE INSERT ON items FOR EACH ROW SET NEW.qty = GREATEST(NEW.qty, 1)",
	}
	mysqlTarget := []string{
		"CREATE TABLE orders (id int NOT NULL AUTO_INCREMENT PRIMARY KEY, customer varchar(20) NOT NULL, " +
			"created timestamp NULL DEFAULT CURRENT_TIMESTAMP, KEY ix_old (created))",
		"CREATE TABLE items (id int NOT NULL AUTO_INCREMENT PRIMARY KEY, order_id int NOT NULL, qty int NOT NULL DEFAULT 1, " +
			"CONSTRAINT chk_qty CHECK (qty >= 0))",
		"CREATE TABLE legacy (id int PRIMARY KEY)",
		"CREATE VIEW v_orders AS SELECT id FROM orders",
		"CREATE PROCEDURE p_count() BEGIN SELECT 1; END",
	}
	pgSource := []string{
		"CREATE TYPE mood AS ENUM ('sad', 'ok', 'happy')",
		"CREATE TABLE orders (id serial PRIMARY KEY, customer varchar(50) NOT NULL DEFAULT 'x', note text, m mood, created timestamptz DEFAULT now())",
		"CREATE INDEX ix_customer ON orders (lower(customer))",
		"CREATE TABLE items (id int GENERATED ALWAYS AS IDENTITY PRIMARY KEY, order_id int NOT NULL REFERENCES orders(id) ON DELETE CASCADE, " +
			"qty int NOT NULL DEFAULT 1 CHECK (qty > 0), total numeric GENERATED ALWAYS AS (qty * 2) STORED)",
		"CREATE TABLE tags (id bigserial PRIMARY KEY, name text UNIQUE)",
		"CREATE VIEW v_orders AS SELECT id, customer FROM orders",
		"CREATE FUNCTION f_qty() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.qty := greatest(NEW.qty, 1); RETURN NEW; END $$",
		"CREATE TRIGGER trg_items BEFORE INSERT ON items FOR EACH ROW EXECUTE FUNCTION f_qty()",
	}
	pgTarget := []string{
		"CREATE TYPE mood AS ENUM ('sad', 'happy')",
		"CREATE TABLE orders (id serial PRIMARY KEY, customer varchar(20), m mood, created timestamptz)",
		"CREATE INDEX ix_old ON orders (created)",
		"CREATE TABLE items (id int PRIMARY KEY, order_id int NOT NULL, qty int NOT NULL DEFAULT 1, total numeric GENERATED ALWAYS AS (qty * 2) STORED)",
		"CREATE TABLE legacy (id int)",
		"CREATE VIEW v_orders AS SELECT id FROM orders",
	}
	for _, c := range []struct {
		env, user, maint string
		source, target   []string
		eng              interface {
			Admin
			SchemaReader
			Client
		}
	}{
		{"FLEETDOCK_IT_MARIADB", "root", "mysql", mysqlSource, mysqlTarget, &MariaDB{}},
		{"FLEETDOCK_IT_MYSQL", "root", "mysql", mysqlSource, mysqlTarget, &MariaDB{}},
		{"FLEETDOCK_IT_POSTGRES", "postgres", "postgres", pgSource, pgTarget, &Postgres{}},
	} {
		t.Run(c.env, func(t *testing.T) {
			ctx := context.Background()
			root := itParams(t, c.env, c.user)
			waitReady(t, c.eng, root)
			const sa, sb = "fd_it_schema_a", "fd_it_schema_b"
			for db, ddl := range map[string][]string{sa: c.source, sb: c.target} {
				mustQuery(t, c.eng, root, c.maint, "DROP DATABASE IF EXISTS "+db, true)
				mustQuery(t, c.eng, root, c.maint, "CREATE DATABASE "+db, true)
				for _, s := range ddl {
					mustQuery(t, c.eng, root, db, s, true)
				}
			}
			read := func(db string) *schemadiff.Schema {
				t.Helper()
				s, err := c.eng.ReadSchema(ctx, root, db)
				if err != nil {
					t.Fatalf("read %s: %v", db, err)
				}
				return s
			}
			a, b := read(sa), read(sb)
			if a.Hash() == b.Hash() {
				t.Fatal("different schemas hash the same")
			}
			if len(a.Tables) != 3 || len(a.Views) != 1 || len(a.Routines) != 1 || len(a.Triggers) != 1 {
				t.Fatalf("source read as %+v", a.Counts())
			}

			// Data and counters do not change the hash.
			mustQuery(t, c.eng, root, sa, "INSERT INTO orders (customer) VALUES ('a'), ('b')", true)
			if h := read(sa).Hash(); h != a.Hash() {
				t.Fatal("inserting rows changed the hash")
			}
			if d := schemadiff.Diff(a, read(sa)); len(d) != 0 {
				t.Fatalf("diff with itself: %+v", d)
			}

			script := schemadiff.Migrate(b, a)
			if script.Review != 1 || !strings.Contains(script.SQL, "-- REVIEW (drops the table and its rows): DROP TABLE") {
				t.Fatalf("review steps = %d in\n%s", script.Review, script.SQL)
			}
			stmts := SplitStatements(script.SQL)
			if _, err := c.eng.QueryBatch(ctx, root, sb, stmts, 10, true, nil); err != nil {
				t.Fatalf("apply: %v\n%s", err, script.SQL)
			}
			after := read(sb)
			left := schemadiff.Diff(after, a)
			if len(left) != 1 || left[0].Kind != schemadiff.KindTable || left[0].Op != schemadiff.Removed || left[0].Table != "legacy" {
				for _, l := range left {
					t.Logf("%s %s: %q -> %q", l.Op, l.Label(), l.Before, l.After)
				}
				t.Fatalf("after applying, %d differences remain; script:\n%s", len(left), script.SQL)
			}
			// Dropping legacy by hand makes the hashes equal.
			mustQuery(t, c.eng, root, sb, "DROP TABLE legacy", true)
			if read(sb).Hash() != a.Hash() {
				t.Fatal("hashes differ after the full migration")
			}
		})
	}
}
