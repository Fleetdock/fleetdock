//go:build integration

package engine

import (
	"context"
	"errors"
	"testing"
)

func TestIntegrationRowChanges(t *testing.T) {
	for _, c := range []struct {
		env, user, maint, table string
		eng                     interface {
			Admin
			DataEditor
			Client
		}
		ddl []string
	}{
		{"FLEETDOCK_IT_MARIADB", "root", "mysql", "people", &MariaDB{}, []string{
			"CREATE TABLE people (id int AUTO_INCREMENT PRIMARY KEY, name varchar(50) NOT NULL, note text NULL, " +
				"blob_v varbinary(16) NULL, seen timestamp NULL, upper_name varchar(50) AS (UPPER(name)) VIRTUAL)",
		}},
		{"FLEETDOCK_IT_MYSQL", "root", "mysql", "people", &MariaDB{}, []string{
			"CREATE TABLE people (id int AUTO_INCREMENT PRIMARY KEY, name varchar(50) NOT NULL, note text NULL, " +
				"blob_v varbinary(16) NULL, seen timestamp NULL, upper_name varchar(50) AS (UPPER(name)) VIRTUAL)",
		}},
		{"FLEETDOCK_IT_POSTGRES", "postgres", "postgres", "public.people", &Postgres{}, []string{
			"CREATE TABLE people (id int GENERATED ALWAYS AS IDENTITY PRIMARY KEY, name varchar(50) NOT NULL, note text, " +
				"blob_v bytea, seen timestamptz, upper_name text GENERATED ALWAYS AS (upper(name)) STORED)",
		}},
	} {
		t.Run(c.env, func(t *testing.T) {
			ctx := context.Background()
			root := itParams(t, c.env, c.user)
			waitReady(t, c.eng, root)
			const db = "fd_it_rows"
			mustQuery(t, c.eng, root, c.maint, "DROP DATABASE IF EXISTS "+db, true)
			mustQuery(t, c.eng, root, c.maint, "CREATE DATABASE "+db, true)
			for _, s := range c.ddl {
				mustQuery(t, c.eng, root, db, s, true)
			}
			isPG := c.user == "postgres"
			must := func(what string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("%s: %v", what, err)
				}
			}
			seen := "2026-03-01 10:00:00"
			if isPG {
				seen = "2026-03-01 10:00:00+00"
			}
			// Binary with NUL and 0xff: PostgreSQL takes bytea as hex text.
			bin := "\x00\xff\x01"
			if isPG {
				bin = `\x00ff01`
			}

			// Insert without the id: the database assigns it, and the change
			// knows the key anyway.
			ins, err := c.eng.InsertRow(ctx, root, db, c.table, RowValues{"name": sp("ann"), "blob_v": &bin, "seen": &seen})
			must("insert", err)
			if !ins.Undoable || ins.Key["id"] == nil || *ins.After["upper_name"] != "ANN" || len(ins.Session) == 0 {
				t.Fatalf("insert = %+v", ins)
			}
			key := ins.Key

			// Update: the images hold only what changed, plus the key.
			upd, err := c.eng.UpdateRow(ctx, root, db, c.table, key, RowValues{"note": sp("first")})
			must("update", err)
			if len(upd.Before) != 2 || upd.Before["note"] != nil || *upd.After["note"] != "first" {
				t.Fatalf("update = %+v", upd)
			}

			// Undo the update.
			undo, err := c.eng.ApplyRowImage(ctx, root, db, c.table, ApplyRequest{
				Key: upd.Key, Check: ExpectImage(upd.After), Want: upd.Before, Session: upd.Session,
			})
			must("undo update", err)
			if undo.Op != "update" {
				t.Fatalf("undo = %+v", undo)
			}
			res := mustQuery(t, c.eng, root, db, "SELECT note FROM people", false)
			if res.Rows[0][0] != nil {
				t.Fatalf("note after undo = %q", *res.Rows[0][0])
			}

			// A later change makes undo refuse, unless forced.
			_, err = c.eng.UpdateRow(ctx, root, db, c.table, key, RowValues{"note": sp("second")})
			must("update again", err)
			mustQuery(t, c.eng, root, db, "UPDATE people SET note = 'someone else'", true)
			_, err = c.eng.ApplyRowImage(ctx, root, db, c.table, ApplyRequest{
				Key: key, Check: ExpectImage(RowValues{"note": sp("second")}), Want: RowValues{"note": nil},
			})
			if !errors.Is(err, ErrRowChanged) {
				t.Fatalf("conflicting undo err = %v", err)
			}

			// Delete, then undo it: the row comes back byte for byte, with
			// its identity value and timestamp, generated column recomputed.
			del, err := c.eng.DeleteRow(ctx, root, db, c.table, key)
			must("delete", err)
			if del.After != nil || *del.Before["name"] != "ann" {
				t.Fatalf("delete = %+v", del)
			}
			back, err := c.eng.ApplyRowImage(ctx, root, db, c.table, ApplyRequest{
				Key: del.Key, Check: ExpectImage(nil), Want: del.Before, Session: del.Session,
			})
			must("undo delete", err)
			if back.Op != "insert" || *back.After["id"] != *key["id"] || *back.After["upper_name"] != "ANN" {
				t.Fatalf("reinsert = %+v", back)
			}
			if *back.After["blob_v"] != *del.Before["blob_v"] || *back.After["seen"] != *del.Before["seen"] {
				t.Fatalf("values changed: before %q %q, after %q %q",
					*del.Before["blob_v"], *del.Before["seen"], *back.After["blob_v"], *back.After["seen"])
			}

			// Undo the insert (delete it again) after checking it is unchanged.
			gone, err := c.eng.ApplyRowImage(ctx, root, db, c.table, ApplyRequest{
				Key: back.Key, Check: ExpectImage(back.After), Want: nil,
			})
			must("undo insert", err)
			if gone.Op != "delete" {
				t.Fatalf("undo insert = %+v", gone)
			}
			if res := mustQuery(t, c.eng, root, db, "SELECT count(*) FROM people", false); *res.Rows[0][0] != "0" {
				t.Fatalf("rows left = %s", *res.Rows[0][0])
			}

			// Browsing reports the generated column as such.
			br, err := c.eng.BrowseRows(ctx, root, db, BrowseRequest{Table: c.table, Limit: 1})
			must("browse", err)
			for _, col := range br.Columns {
				if (col.Name == "upper_name") != col.Generated {
					t.Errorf("column %s generated = %v", col.Name, col.Generated)
				}
			}
		})
	}
}
