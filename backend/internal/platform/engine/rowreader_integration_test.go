//go:build integration

package engine

import (
	"context"
	"testing"
)

func TestIntegrationRowReader(t *testing.T) {
	for _, c := range []struct {
		env, user, maint, table, ddl string
		eng                          interface {
			Admin
			RowReader
			Client
		}
	}{
		{"FLEETDOCK_IT_MARIADB", "root", "mysql", "rows_x",
			"CREATE TABLE rows_x (region varchar(10) COLLATE utf8mb4_general_ci, num int, seen timestamp NULL, v text, PRIMARY KEY (region, num))", &MariaDB{}},
		{"FLEETDOCK_IT_MYSQL", "root", "mysql", "rows_x",
			"CREATE TABLE rows_x (region varchar(10) COLLATE utf8mb4_0900_ai_ci, num int, seen timestamp NULL, v text, PRIMARY KEY (region, num))", &MariaDB{}},
		{"FLEETDOCK_IT_POSTGRES", "postgres", "postgres", "public.rows_x",
			`CREATE TABLE rows_x (region varchar(10), num int, seen timestamptz, v text, PRIMARY KEY (region, num))`, &Postgres{}},
	} {
		t.Run(c.env, func(t *testing.T) {
			ctx := context.Background()
			root := itParams(t, c.env, c.user)
			waitReady(t, c.eng, root)
			const db = "fd_it_reader"
			mustQuery(t, c.eng, root, c.maint, "DROP DATABASE IF EXISTS "+db, true)
			mustQuery(t, c.eng, root, c.maint, "CREATE DATABASE "+db, true)
			mustQuery(t, c.eng, root, db, c.ddl, true)
			// Mixed case on a case-insensitive key: the database's order,
			// not Go's byte order, must drive the scan.
			mustQuery(t, c.eng, root, db, "INSERT INTO rows_x (region, num, v) VALUES "+
				"('b', 1, 'x'), ('A', 2, 'y'), ('a', 1, NULL), ('C', 1, 'z'), ('b', 2, 'w')", true)

			var all []RowValues
			var after RowValues
			pages := 0
			for {
				p, err := c.eng.ScanRows(ctx, root, db, c.table, after, 2)
				if err != nil {
					t.Fatalf("scan: %v", err)
				}
				pages++
				all = append(all, p.Rows...)
				if p.Next == nil {
					break
				}
				after = p.Next
				if pages > 10 {
					t.Fatal("scan does not end")
				}
			}
			if len(all) != 5 {
				t.Fatalf("scanned %d rows in %d pages", len(all), pages)
			}
			seen := map[string]bool{}
			for _, r := range all {
				k := KeyString([]string{"region", "num"}, r)
				if seen[k] {
					t.Fatalf("row %v seen twice", r)
				}
				seen[k] = true
			}

			got, err := c.eng.GetRows(ctx, root, db, c.table, []RowValues{
				{"region": sp("b"), "num": sp("2")}, {"region": sp("zz"), "num": sp("9")},
			})
			if err != nil || len(got.Rows) != 1 || *got.Rows[0]["v"] != "w" || len(got.Columns) != 4 {
				t.Fatalf("get = %+v, %v", got, err)
			}
			none, err := c.eng.GetRows(ctx, root, db, c.table, nil)
			if err != nil || len(none.Rows) != 0 || len(none.Columns) != 4 {
				t.Fatalf("get none = %+v, %v", none, err)
			}
		})
	}
}
