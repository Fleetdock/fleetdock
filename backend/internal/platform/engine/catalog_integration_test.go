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
			if len(byName) != 4 || cat.Truncated {
				t.Fatalf("tables = %+v, truncated %v", cat.Tables, cat.Truncated)
			}
			orders := byName["orders"]
			if orders.Kind != "table" || len(orders.Columns) != 3 || orders.Columns[0].Name != "region" ||
				orders.Columns[1].Name != "num" || orders.Columns[2].Name != "note" {
				t.Errorf("orders = %+v", orders)
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
			none, err := c.eng.ReferencedBy(ctx, root, db, map[bool]string{true: "public.notes", false: "notes"}[isPG])
			if err != nil || len(none) != 0 {
				t.Errorf("notes incoming = %+v, %v", none, err)
			}
		})
	}
}
