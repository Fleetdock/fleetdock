//go:build integration

package engine

import (
	"context"
	"testing"
)

// catalogEngines are the engines under integration test with their admin
// login and maintenance database.
func catalogEngines() []struct {
	env, user, maint string
	eng              interface {
		Admin
		Catalog
		Client
	}
} {
	return []struct {
		env, user, maint string
		eng              interface {
			Admin
			Catalog
			Client
		}
	}{
		{"FLEETDOCK_IT_MARIADB", "root", "mysql", &MariaDB{}},
		{"FLEETDOCK_IT_MYSQL", "root", "mysql", &MariaDB{}},
		{"FLEETDOCK_IT_POSTGRES", "postgres", "postgres", &Postgres{}},
	}
}

func TestIntegrationCatalog(t *testing.T) {
	for _, c := range catalogEngines() {
		t.Run(c.env, func(t *testing.T) {
			ctx := context.Background()
			root := itParams(t, c.env, c.user)
			waitReady(t, c.eng, root)
			const db = "fd_it_catalog"
			mustQuery(t, c.eng, root, c.maint, "DROP DATABASE IF EXISTS "+db, true)
			mustQuery(t, c.eng, root, c.maint, "CREATE DATABASE "+db, true)
			isPG := c.user == "postgres"
			for _, stmt := range []string{
				"CREATE TABLE orders (region varchar(10) NOT NULL, num int NOT NULL, note text, PRIMARY KEY (region, num))",
				"CREATE TABLE items (id int PRIMARY KEY, o_region varchar(10), o_num int, " +
					"CONSTRAINT fk_items_order FOREIGN KEY (o_region, o_num) REFERENCES orders (region, num))",
				"CREATE TABLE notes (id int PRIMARY KEY, region varchar(10), num int, " +
					"CONSTRAINT fk_notes_order FOREIGN KEY (region, num) REFERENCES orders (region, num))",
				"CREATE VIEW open_orders AS SELECT region, num FROM orders",
				"CREATE TABLE staff (id int PRIMARY KEY, manager_id int, " +
					"CONSTRAINT fk_staff_manager FOREIGN KEY (manager_id) REFERENCES staff (id) ON DELETE SET NULL)",
			} {
				mustQuery(t, c.eng, root, db, stmt, true)
			}

			cat, err := c.eng.Columns(ctx, root, db)
			if err != nil {
				t.Fatalf("columns: %v", err)
			}
			byName := map[string]CatalogTable{}
			for _, tb := range cat.Tables {
				byName[tb.Name] = tb
			}
			if len(byName) != 5 || cat.Truncated {
				t.Fatalf("tables = %+v, truncated %v", cat.Tables, cat.Truncated)
			}
			orders := byName["orders"]
			if orders.Kind != "table" || len(orders.Columns) != 3 || orders.Columns[0].Name != "region" ||
				orders.Columns[1].Name != "num" || orders.Columns[2].Name != "note" {
				t.Errorf("orders = %+v", orders)
			}
			if c := orders.Columns; !c[0].PrimaryKey || !c[1].PrimaryKey || c[2].PrimaryKey ||
				c[0].Nullable || !c[2].Nullable {
				t.Errorf("orders keys/nullability = %+v", c)
			}
			if byName["open_orders"].Kind != "view" {
				t.Errorf("open_orders kind = %q", byName["open_orders"].Kind)
			}
			if isPG && orders.Schema != "public" {
				t.Errorf("schema = %q", orders.Schema)
			}

			table := "orders"
			if isPG {
				table = "public.orders"
			}
			fks, err := c.eng.ReferencedBy(ctx, root, db, table)
			if err != nil {
				t.Fatalf("referenced by: %v", err)
			}
			if len(fks) != 2 {
				t.Fatalf("incoming = %+v", fks)
			}
			want := "items"
			if isPG {
				want = "public.items"
			}
			items := fks[0]
			if items.Table != want || items.Name != "fk_items_order" ||
				len(items.Columns) != 2 || items.Columns[0] != "o_region" || items.Columns[1] != "o_num" ||
				items.RefColumns[0] != "region" || items.RefColumns[1] != "num" {
				t.Errorf("items fk = %+v", items)
			}
			q := func(n string) string {
				if isPG {
					return "public." + n
				}
				return n
			}
			rels, truncated, err := c.eng.Relations(ctx, root, db)
			if err != nil || truncated {
				t.Fatalf("relations: %v, truncated %v", err, truncated)
			}
			if len(rels) != 3 {
				t.Fatalf("relations = %+v", rels)
			}
			// Ordered by referencing table, then constraint name.
			if r := rels[0]; r.Table != q("items") || r.Name != "fk_items_order" || r.RefTable != q("orders") ||
				len(r.Columns) != 2 || r.Columns[1] != "o_num" || r.RefColumns[1] != "num" {
				t.Errorf("items relation = %+v", r)
			}
			if r := rels[2]; r.Table != q("staff") || r.RefTable != q("staff") || r.Columns[0] != "manager_id" ||
				r.RefColumns[0] != "id" || r.OnDelete != "SET NULL" {
				t.Errorf("self relation = %+v", r)
			}
			if isPG {
				// Keys across schemas, and none duplicated onto partitions.
				for _, stmt := range []string{
					"CREATE SCHEMA billing",
					"CREATE TABLE billing.invoices (id int PRIMARY KEY, staff_id int REFERENCES public.staff (id))",
					"CREATE TABLE billing.lines (inv int REFERENCES billing.invoices (id), at date NOT NULL) PARTITION BY RANGE (at)",
					"CREATE TABLE billing.lines_2026 PARTITION OF billing.lines FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')",
				} {
					mustQuery(t, c.eng, root, db, stmt, true)
				}
				rels, _, err = c.eng.Relations(ctx, root, db)
				if err != nil || len(rels) != 5 {
					t.Fatalf("relations with billing = %+v, %v", rels, err)
				}
				if rels[0].Table != "billing.invoices" || rels[0].RefTable != "public.staff" ||
					rels[1].Table != "billing.lines" || rels[1].RefTable != "billing.invoices" {
					t.Errorf("cross-schema relations = %+v", rels[:2])
				}

				// Limited to one schema: its tables, and the keys that cross into it.
				billing, err := c.eng.Columns(ctx, root, db, "billing")
				if err != nil || len(billing.Tables) != 2 || billing.Tables[0].Schema != "billing" {
					t.Errorf("billing columns = %+v, %v", billing, err)
				}
				rels, _, err = c.eng.Relations(ctx, root, db, "billing")
				if err != nil || len(rels) != 2 {
					t.Errorf("billing relations = %+v, %v", rels, err)
				}
				pub, _, err := c.eng.Relations(ctx, root, db, "public")
				if err != nil || len(pub) != 4 {
					// items, notes, staff (self) and billing.invoices -> public.staff
					t.Errorf("public relations = %+v, %v", pub, err)
				}
				counts, err := c.eng.SchemaCounts(ctx, root, db)
				if err != nil || len(counts) != 2 || counts[0].Name != "billing" || counts[0].Tables != 2 ||
					counts[1].Name != "public" || counts[1].Tables != 4 || counts[1].Views != 1 {
					t.Errorf("schema counts = %+v, %v", counts, err)
				}
			} else if counts, err := c.eng.SchemaCounts(ctx, root, db); err != nil || counts != nil {
				t.Errorf("mysql schema counts = %+v, %v", counts, err)
			}
			none, err := c.eng.ReferencedBy(ctx, root, db, map[bool]string{true: "public.notes", false: "notes"}[isPG])
			if err != nil || len(none) != 0 {
				t.Errorf("notes incoming = %+v, %v", none, err)
			}
		})
	}
}
