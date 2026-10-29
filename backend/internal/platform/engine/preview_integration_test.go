//go:build integration

package engine

import (
	"context"
	"strings"
	"testing"
)

func TestIntegrationPreview(t *testing.T) {
	for _, c := range []struct {
		env, user, maint string
		eng              interface {
			Admin
			Previewer
			Client
		}
	}{
		{"FLEETDOCK_IT_MARIADB", "root", "mysql", &MariaDB{}},
		{"FLEETDOCK_IT_MYSQL", "root", "mysql", &MariaDB{}},
		{"FLEETDOCK_IT_POSTGRES", "postgres", "postgres", &Postgres{}},
	} {
		t.Run(c.env, func(t *testing.T) {
			ctx := context.Background()
			root := itParams(t, c.env, c.user)
			waitReady(t, c.eng, root)
			const db = "fd_it_preview"
			mustQuery(t, c.eng, root, c.maint, "DROP DATABASE IF EXISTS "+db, true)
			mustQuery(t, c.eng, root, c.maint, "CREATE DATABASE "+db, true)
			mustQuery(t, c.eng, root, db, "CREATE TABLE items (id int PRIMARY KEY, sku varchar(10), qty int)", true)
			mustQuery(t, c.eng, root, db, "INSERT INTO items VALUES (1, 'a', 1), (2, 'b', 2), (3, 'c', 3)", true)
			isPG := c.user == "postgres"

			res, err := c.eng.PreviewBatch(ctx, root, db, []string{
				"UPDATE items i SET qty = qty * 10 WHERE i.qty >= 2",
				"SELECT sum(qty) FROM items",
				"DELETE FROM items WHERE id = 1",
				"INSERT INTO items VALUES (9, 'z', 9)",
			}, 100, nil)
			if err != nil {
				t.Fatalf("preview: %v", err)
			}
			if len(res) != 4 {
				t.Fatalf("results = %d", len(res))
			}
			upd := res[0]
			if upd.RowsAffected != 2 || upd.Sample == nil || len(upd.Sample.Rows) != 2 || len(upd.Notes) == 0 {
				t.Fatalf("update = %+v", upd)
			}
			qtyCol := -1
			for i, col := range upd.Sample.Columns {
				if col == "qty" {
					qtyCol = i
				}
			}
			r := upd.Sample.Rows[0]
			if *r.Before[qtyCol] != "2" || r.After == nil || *r.After[qtyCol] != "20" {
				t.Fatalf("sample row = %v → %v", r.Before, r.After)
			}
			// A read later in the script sees the preview's own changes.
			if got := *res[1].Rows[0][0]; got != "51" {
				t.Fatalf("sum inside preview = %s", got)
			}
			del := res[2]
			if del.RowsAffected != 1 || del.Sample == nil || len(del.Sample.Rows) != 1 || del.Sample.Rows[0].After != nil {
				t.Fatalf("delete = %+v", del)
			}
			if res[3].RowsAffected != 1 || res[3].Sample != nil {
				t.Fatalf("insert = %+v", res[3])
			}
			// Nothing was saved.
			if got := *mustQuery(t, c.eng, root, db, "SELECT sum(qty) FROM items", false).Rows[0][0]; got != "6" {
				t.Fatalf("sum after preview = %s", got)
			}
			if got := *mustQuery(t, c.eng, root, db, "SELECT count(*) FROM items", false).Rows[0][0]; got != "3" {
				t.Fatalf("count after preview = %s", got)
			}

			// A failure part-way leaves nothing behind either.
			if _, err := c.eng.PreviewBatch(ctx, root, db, []string{
				"UPDATE items SET qty = 100", "INSERT INTO items VALUES (1, 'dup', 1)",
			}, 100, nil); err == nil {
				t.Fatal("duplicate key should fail")
			}
			if got := *mustQuery(t, c.eng, root, db, "SELECT max(qty) FROM items", false).Rows[0][0]; got != "3" {
				t.Fatalf("max after failed preview = %s", got)
			}

			// DDL and transaction control are refused before anything runs.
			for _, bad := range []string{"DROP TABLE items", "COMMIT"} {
				if _, err := c.eng.PreviewBatch(ctx, root, db, []string{"DELETE FROM items", bad}, 100, nil); err == nil {
					t.Fatalf("%q must be refused", bad)
				}
			}
			if got := *mustQuery(t, c.eng, root, db, "SELECT count(*) FROM items", false).Rows[0][0]; got != "3" {
				t.Fatalf("count after refused preview = %s", got)
			}

			if !isPG {
				mustQuery(t, c.eng, root, db, "CREATE TABLE legacy (id int PRIMARY KEY) ENGINE=MyISAM", true)
				_, err := c.eng.PreviewBatch(ctx, root, db, []string{"DELETE FROM items"}, 100, nil)
				if err == nil || !strings.Contains(err.Error(), "legacy (MyISAM)") {
					t.Fatalf("MyISAM table must block preview, got %v", err)
				}
			}
		})
	}
}
