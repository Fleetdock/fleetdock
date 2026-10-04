//go:build integration

// Integration tests for the least-privilege console roles against real
// engines. Each engine is tested only when its address is set:
//
//	FLEETDOCK_IT_MARIADB=127.0.0.1:33061  (root password in FLEETDOCK_IT_PASSWORD)
//	FLEETDOCK_IT_MYSQL=127.0.0.1:33062
//	FLEETDOCK_IT_POSTGRES=127.0.0.1:54329
//
//	go test -tags integration ./internal/platform/engine/ -run Integration
package engine

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func itParams(t *testing.T, env, user string) ConnParams {
	t.Helper()
	addr := os.Getenv(env)
	if addr == "" {
		t.Skipf("%s not set", env)
	}
	host, ps, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(ps)
	pw := os.Getenv("FLEETDOCK_IT_PASSWORD")
	if pw == "" {
		pw = "rootpw"
	}
	return ConnParams{Host: host, Port: port, User: user, Password: pw, TLSMode: TLSDisable}
}

// waitReady retries Ping while the container finishes initialising.
func waitReady(t *testing.T, c Client, p ConnParams) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, err := c.Ping(ctx, p)
		cancel()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("engine never became ready: %v", err)
		}
		time.Sleep(time.Second)
	}
}

func mustQuery(t *testing.T, a Admin, p ConnParams, db, sql string, write bool) *QueryResult {
	t.Helper()
	res, err := a.Query(context.Background(), p, db, sql, 100, write)
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return res
}

func mustFail(t *testing.T, a Admin, p ConnParams, db, sql string, write bool) {
	t.Helper()
	if _, err := a.Query(context.Background(), p, db, sql, 100, write); err == nil {
		t.Fatalf("%q as %s should have been refused", sql, p.User)
	}
}

func TestIntegrationMySQLFamilyConsoleRoles(t *testing.T) {
	for _, env := range []string{"FLEETDOCK_IT_MARIADB", "FLEETDOCK_IT_MYSQL"} {
		t.Run(env, func(t *testing.T) {
			ctx := context.Background()
			m := &MariaDB{}
			root := itParams(t, env, "root")
			waitReady(t, m, root)

			for _, stmt := range []string{
				"DROP DATABASE IF EXISTS fd_it_app", "DROP DATABASE IF EXISTS fd_it_other",
				"CREATE DATABASE fd_it_app", "CREATE DATABASE fd_it_other",
				"CREATE TABLE fd_it_app.t (id int primary key, v text)",
				"INSERT INTO fd_it_app.t VALUES (1, 'a')",
				"CREATE TABLE fd_it_other.secret (id int)",
				"DROP USER IF EXISTS 'fd_it_ro'@'%'", "DROP USER IF EXISTS 'fd_it_rw'@'%'",
			} {
				if _, err := m.Query(ctx, root, "mysql", stmt, 1, true); err != nil {
					t.Fatalf("setup %q: %v", stmt, err)
				}
			}

			for _, u := range []string{"fd_it_ro", "fd_it_rw"} {
				if err := m.CreateDBUser(ctx, root, u, "%", "Role-pw-123"); err != nil {
					t.Fatalf("create %s: %v", u, err)
				}
				if err := ApplyConsoleProfile(ctx, m, root, u, "%", "fd_it_app", u == "fd_it_rw"); err != nil {
					t.Fatalf("profile %s: %v", u, err)
				}
			}
			ro, rw := root, root
			ro.User, ro.Password = "fd_it_ro", "Role-pw-123"
			rw.User, rw.Password = "fd_it_rw", "Role-pw-123"

			// Controls: root can do what the roles must not, so the refusals
			// below are the roles' doing, not broken SQL.
			mustQuery(t, m, root, "fd_it_app", "SELECT * FROM fd_it_other.secret", false)
			mustQuery(t, m, root, "fd_it_app", "SELECT * FROM mysql.user", false)

			// Read role: its own database only.
			if res := mustQuery(t, m, ro, "fd_it_app", "SELECT v FROM t", false); res.RowCount != 1 {
				t.Errorf("ro read = %d rows, want 1", res.RowCount)
			}
			mustFail(t, m, ro, "fd_it_app", "SELECT * FROM fd_it_other.secret", false)
			mustFail(t, m, ro, "fd_it_app", "SELECT * FROM mysql.user", false)
			mustFail(t, m, ro, "fd_it_app", "INSERT INTO t VALUES (2, 'b')", true)
			if res, err := m.Query(ctx, ro, "fd_it_app", "SELECT LOAD_FILE('/etc/passwd')", 1, false); err == nil &&
				len(res.Rows) == 1 && res.Rows[0][0] != nil {
				t.Error("LOAD_FILE must not return file contents to the console role")
			}
			dbs := mustQuery(t, m, ro, "fd_it_app", "SHOW DATABASES", false)
			for _, row := range dbs.Rows {
				if row[0] != nil && *row[0] == "fd_it_other" {
					t.Error("ro role can see fd_it_other")
				}
			}

			// Write role: data and DDL in its database, nothing beyond.
			mustQuery(t, m, rw, "fd_it_app", "INSERT INTO t VALUES (2, 'b')", true)
			mustQuery(t, m, rw, "fd_it_app", "CREATE TABLE t2 (id int)", true)
			mustQuery(t, m, rw, "fd_it_app", "ALTER TABLE t2 ADD COLUMN c int", true)
			mustFail(t, m, rw, "fd_it_app", "DROP DATABASE fd_it_other", true)
			mustFail(t, m, rw, "fd_it_app", "DELETE FROM fd_it_other.secret", true)
			mustFail(t, m, rw, "fd_it_app", "CREATE USER 'evil'@'%' IDENTIFIED BY 'x'", true)
			mustFail(t, m, rw, "fd_it_app", "SET GLOBAL max_connections = 1", true)
			mustFail(t, m, rw, "fd_it_app", "SELECT 1 INTO OUTFILE '/tmp/fd_it_out'", true)

			// Server-side statement timeout.
			slow := ro
			slow.StatementTimeout = time.Second
			start := time.Now()
			_, err := m.Query(ctx, slow, "fd_it_app", "SELECT SLEEP(5)", 1, false)
			// MariaDB aborts with an error; MySQL's max_execution_time
			// interrupts SLEEP and returns 1. Either way it must stop early.
			if time.Since(start) > 4*time.Second {
				t.Errorf("statement ran %s despite a 1s timeout (err=%v)", time.Since(start), err)
			}
		})
	}
}

func TestIntegrationPostgresConsoleRoles(t *testing.T) {
	ctx := context.Background()
	pg := &Postgres{}
	root := itParams(t, "FLEETDOCK_IT_POSTGRES", "postgres")
	waitReady(t, pg, root)

	run := func(db, stmt string) {
		t.Helper()
		if _, err := pg.Query(ctx, root, db, stmt, 1, true); err != nil {
			t.Fatalf("setup %q: %v", stmt, err)
		}
	}
	for _, stmt := range []string{
		"DROP DATABASE IF EXISTS fd_it_app", "DROP DATABASE IF EXISTS fd_it_other",
		"DROP ROLE IF EXISTS fd_it_ro", "DROP ROLE IF EXISTS fd_it_rw", "DROP ROLE IF EXISTS fd_it_owner",
		"CREATE ROLE fd_it_owner", "CREATE DATABASE fd_it_app OWNER fd_it_owner", "CREATE DATABASE fd_it_other",
	} {
		run("postgres", stmt)
	}
	run("fd_it_app", "SET ROLE fd_it_owner; CREATE TABLE t (id int primary key, v text); INSERT INTO t VALUES (1, 'a'); CREATE SCHEMA app AUTHORIZATION fd_it_owner; CREATE TABLE app.x (id int); RESET ROLE")
	run("fd_it_other", "CREATE TABLE secret (id int)")

	for _, u := range []string{"fd_it_ro", "fd_it_rw"} {
		if err := pg.CreateDBUser(ctx, root, u, "", "Role-pw-123"); err != nil {
			t.Fatalf("create %s: %v", u, err)
		}
		if err := ApplyConsoleProfile(ctx, pg, root, u, "", "fd_it_app", u == "fd_it_rw"); err != nil {
			t.Fatalf("profile %s: %v", u, err)
		}
	}
	ro, rw := root, root
	ro.User, ro.Password = "fd_it_ro", "Role-pw-123"
	rw.User, rw.Password = "fd_it_rw", "Role-pw-123"

	// Controls (see the MySQL test).
	mustQuery(t, pg, root, "fd_it_app", "SELECT * FROM pg_authid", false)
	mustQuery(t, pg, root, "fd_it_other", "SELECT * FROM secret", false)

	if res := mustQuery(t, pg, ro, "fd_it_app", "SELECT v FROM t", false); res.RowCount != 1 {
		t.Errorf("ro read = %d rows, want 1", res.RowCount)
	}
	mustQuery(t, pg, ro, "fd_it_app", "SELECT * FROM app.x", false)
	mustFail(t, pg, ro, "fd_it_app", "SELECT * FROM pg_authid", false)
	mustFail(t, pg, ro, "fd_it_app", "SELECT pg_read_file('/etc/passwd')", false)
	mustFail(t, pg, ro, "fd_it_app", "INSERT INTO t VALUES (2, 'b')", true)
	// default_transaction_read_only: even a "write" path refuses.
	mustFail(t, pg, ro, "fd_it_app", "CREATE TABLE sneaky (id int)", true)

	mustQuery(t, pg, rw, "fd_it_app", "INSERT INTO t VALUES (2, 'b')", true)
	// Owner membership lets the write role alter tables it does not own.
	mustQuery(t, pg, rw, "fd_it_app", "ALTER TABLE t ADD COLUMN c int", true)
	mustFail(t, pg, rw, "fd_it_app", "DROP DATABASE fd_it_other", true)
	mustFail(t, pg, rw, "fd_it_app", "CREATE ROLE evil LOGIN", true)
	mustFail(t, pg, rw, "fd_it_app", "ALTER SYSTEM SET work_mem = '1GB'", true)
	mustFail(t, pg, rw, "fd_it_app", "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename = 'postgres'", false)

	// A table the write role creates later is not covered by the read role's
	// grants. That must surface as a permission error (which the service
	// repairs by re-applying grants), never as "table not found".
	mustQuery(t, pg, rw, "fd_it_app", "CREATE TABLE later_table (id int primary key)", true)
	_, err := pg.BrowseRows(ctx, ro, "fd_it_app", BrowseRequest{Table: "public.later_table"})
	if !IsPermissionDenied(err) {
		t.Fatalf("browsing an ungranted table: err = %v, want a permission error", err)
	}
	if err := ApplyConsoleProfile(ctx, pg, root, "fd_it_ro", "", "fd_it_app", false); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.BrowseRows(ctx, ro, "fd_it_app", BrowseRequest{Table: "public.later_table"}); err != nil {
		t.Fatalf("after re-applying grants: %v", err)
	}

	// An owner of several databases is never delegated: membership would let
	// the console reach the others.
	run("postgres", "DROP DATABASE IF EXISTS fd_it_app2")
	run("postgres", "CREATE DATABASE fd_it_app2 OWNER fd_it_owner")
	run("postgres", "DROP ROLE IF EXISTS fd_it_rw2")
	if err := pg.CreateDBUser(ctx, root, "fd_it_rw2", "", "Role-pw-123"); err != nil {
		t.Fatal(err)
	}
	if err := ApplyConsoleProfile(ctx, pg, root, "fd_it_rw2", "", "fd_it_app", true); err != nil {
		t.Fatal(err)
	}
	var member bool
	res := mustQuery(t, pg, root, "postgres", "SELECT pg_has_role('fd_it_rw2', 'fd_it_owner', 'MEMBER')::text", false)
	member = res.Rows[0][0] != nil && *res.Rows[0][0] == "true"
	if member {
		t.Error("write role must not join an owner that owns more than one database")
	}
	// fd_it_rw joined earlier, while the owner had one database; re-applying
	// now must withdraw that.
	if err := ApplyConsoleProfile(ctx, pg, root, "fd_it_rw", "", "fd_it_app", true); err != nil {
		t.Fatal(err)
	}
	res = mustQuery(t, pg, root, "postgres", "SELECT pg_has_role('fd_it_rw', 'fd_it_owner', 'MEMBER')::text", false)
	if *res.Rows[0][0] == "true" {
		t.Error("re-apply must revoke a delegation that is no longer safe")
	}
	run("postgres", "DROP DATABASE fd_it_app2")
	if err := ApplyConsoleProfile(ctx, pg, root, "fd_it_rw", "", "fd_it_app", true); err != nil {
		t.Fatal(err)
	}

	// Separate login: the role must not reach the other database at all.
	if _, err := pg.Query(ctx, ro, "fd_it_other", "SELECT * FROM secret", 1, false); err == nil {
		t.Error("ro role read fd_it_other")
	}

	slow := ro
	slow.StatementTimeout = time.Second
	start := time.Now()
	if _, err := pg.Query(ctx, slow, "fd_it_app", "SELECT pg_sleep(5)", 1, false); err == nil ||
		!strings.Contains(err.Error(), "statement timeout") {
		t.Errorf("pg_sleep(5) with 1s timeout: err=%v", err)
	}
	if time.Since(start) > 4*time.Second {
		t.Errorf("statement ran %s despite a 1s timeout", time.Since(start))
	}

	// SCRAM verifier login works end to end.
	if err := RotatePassword(ctx, pg, root, "fd_it_ro", "", "New-pw-456"); err != nil {
		t.Fatal(err)
	}
	ro.Password = "New-pw-456"
	mustQuery(t, pg, ro, "fd_it_app", "SELECT 1", false)
}

// TestIntegrationMonitor lists sessions, kills a long-running statement and
// reads sizes on every configured engine.
func TestIntegrationMonitor(t *testing.T) {
	cases := []struct {
		env, user, sleep string
		eng              interface {
			Admin
			Monitor
			Client
		}
	}{
		{"FLEETDOCK_IT_MARIADB", "root", "SELECT SLEEP(30)", &MariaDB{}},
		{"FLEETDOCK_IT_MYSQL", "root", "SELECT SLEEP(30)", &MariaDB{}},
		{"FLEETDOCK_IT_POSTGRES", "postgres", "SELECT pg_sleep(30)", &Postgres{}},
	}
	for _, tc := range cases {
		t.Run(tc.env, func(t *testing.T) {
			ctx := context.Background()
			root := itParams(t, tc.env, tc.user)
			waitReady(t, tc.eng, root)
			dbName := "postgres"
			if tc.user == "root" {
				dbName = "mysql"
			}

			done := make(chan error, 1)
			start := time.Now()
			go func() {
				_, err := tc.eng.Query(ctx, root, dbName, tc.sleep, 1, false)
				done <- err
			}()

			var target int64
			for i := 0; i < 50 && target == 0; i++ {
				time.Sleep(100 * time.Millisecond)
				procs, err := tc.eng.Processes(ctx, root)
				if err != nil {
					t.Fatalf("Processes: %v", err)
				}
				for _, p := range procs {
					if p.Query != nil && strings.Contains(strings.ToLower(*p.Query), "sleep(30)") {
						target = p.ID
					}
				}
			}
			if target == 0 {
				t.Fatal("sleeping session not found in process list")
			}
			if err := tc.eng.KillProcess(ctx, root, target, false); err != nil {
				t.Fatalf("KillProcess: %v", err)
			}
			select {
			case <-done:
				if time.Since(start) > 10*time.Second {
					t.Errorf("killed statement still ran %s", time.Since(start))
				}
			case <-time.After(15 * time.Second):
				t.Fatal("statement was not cancelled")
			}

			stats, err := tc.eng.DatabaseStats(ctx, root)
			if err != nil || len(stats) == 0 {
				t.Fatalf("DatabaseStats = %v, %v", stats, err)
			}
			if st, err := tc.eng.ServerStatus(ctx, root); err != nil || len(st) == 0 {
				t.Errorf("ServerStatus = %d items, %v", len(st), err)
			}
			if vars, err := tc.eng.Variables(ctx, root); err != nil || len(vars) < 10 {
				t.Errorf("Variables = %d items, %v", len(vars), err)
			}
		})
	}
}

// TestIntegrationDataEditing exercises browse/insert/update/delete/import on
// every configured engine.
func TestIntegrationDataEditing(t *testing.T) {
	type tc struct {
		env, user, setupDB, table string
		eng                       interface {
			Admin
			DataEditor
			Client
		}
		ddl []string
	}
	cases := []tc{
		{"FLEETDOCK_IT_MARIADB", "root", "mysql", "people", &MariaDB{}, []string{
			"DROP DATABASE IF EXISTS fd_it_data", "CREATE DATABASE fd_it_data",
			"CREATE TABLE fd_it_data.people (id int auto_increment primary key, name varchar(40), age int, note text)",
			"CREATE TABLE fd_it_data.nokey (a int, b int)",
		}},
		{"FLEETDOCK_IT_MYSQL", "root", "mysql", "people", &MariaDB{}, []string{
			"DROP DATABASE IF EXISTS fd_it_data", "CREATE DATABASE fd_it_data",
			"CREATE TABLE fd_it_data.people (id int auto_increment primary key, name varchar(40), age int, note text)",
			"CREATE TABLE fd_it_data.nokey (a int, b int)",
		}},
		{"FLEETDOCK_IT_POSTGRES", "postgres", "postgres", "public.people", &Postgres{}, nil},
	}
	for _, c := range cases {
		t.Run(c.env, func(t *testing.T) {
			ctx := context.Background()
			root := itParams(t, c.env, c.user)
			waitReady(t, c.eng, root)
			if c.ddl == nil {
				mustQuery(t, c.eng, root, "postgres", "DROP DATABASE IF EXISTS fd_it_data", true)
				mustQuery(t, c.eng, root, "postgres", "CREATE DATABASE fd_it_data", true)
				mustQuery(t, c.eng, root, "fd_it_data",
					"CREATE TABLE people (id serial primary key, name varchar(40), age int, note text); CREATE TABLE nokey (a int, b int)", true)
			}
			for _, s := range c.ddl {
				mustQuery(t, c.eng, root, c.setupDB, s, true)
			}
			nokey := "nokey"
			if c.ddl == nil {
				nokey = "public.nokey"
			}

			// Import, including a quote-laden value and NULL.
			csvData := "name,age,note\nann,31,\"says \"\"hi\"\"; DROP TABLE people; --\"\nbob,42,\\N\ncid,7,\n"
			n, err := c.eng.ImportCSV(ctx, root, "fd_it_data", c.table, strings.NewReader(csvData), ImportOptions{EmptyAsNull: true})
			if err != nil || n != 3 {
				t.Fatalf("ImportCSV = %d, %v", n, err)
			}
			// A failing row rolls back the whole import.
			if _, err := c.eng.ImportCSV(ctx, root, "fd_it_data", c.table,
				strings.NewReader("name,age\nzed,1\nbad,notanumber\n"), ImportOptions{}); err == nil {
				t.Error("import with an invalid number should fail")
			}

			res, err := c.eng.BrowseRows(ctx, root, "fd_it_data", BrowseRequest{Table: c.table, Sort: []SortKey{{Column: "age", Desc: true}}})
			if err != nil {
				t.Fatalf("browse: %v", err)
			}
			if len(res.Rows) != 3 || len(res.Key) != 1 || res.Key[0] != "id" {
				t.Fatalf("browse rows=%d key=%v, want 3 rows keyed by id (failed import must not leave rows)", len(res.Rows), res.Key)
			}
			if *res.Rows[0][1] != "bob" {
				t.Errorf("sorted by age desc, first = %v", *res.Rows[0][1])
			}

			// Typed filter (int column) and search.
			age := "30"
			res, err = c.eng.BrowseRows(ctx, root, "fd_it_data", BrowseRequest{Table: c.table,
				Filters: []Filter{{Column: "age", Op: OpGt, Value: &age}}})
			if err != nil {
				t.Fatalf("filter age>30: %v", err)
			}
			if res.Total != 2 || !res.TotalExact {
				t.Fatalf("filter age>30: total=%d exact=%v", res.Total, res.TotalExact)
			}
			res, err = c.eng.BrowseRows(ctx, root, "fd_it_data", BrowseRequest{Table: c.table, Search: "DROP TABLE"})
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(res.Rows) != 1 {
				t.Fatalf("search rows = %d, want 1", len(res.Rows))
			}
			annID := *res.Rows[0][0]

			// Single-row edits.
			newNote := "edited"
			if err := c.eng.UpdateRow(ctx, root, "fd_it_data", c.table, RowValues{"id": &annID}, RowValues{"note": &newNote, "age": nil}); err != nil {
				t.Fatalf("update: %v", err)
			}
			if err := c.eng.UpdateRow(ctx, root, "fd_it_data", c.table, RowValues{"id": &annID}, RowValues{"note": &newNote}); err != nil {
				t.Errorf("re-saving identical values must still match one row: %v", err)
			}
			missing := "999999"
			if err := c.eng.UpdateRow(ctx, root, "fd_it_data", c.table, RowValues{"id": &missing}, RowValues{"note": &newNote}); err == nil {
				t.Error("update of a missing row should fail")
			}
			name, ag := "dee", "55"
			if err := c.eng.InsertRow(ctx, root, "fd_it_data", c.table, RowValues{"name": &name, "age": &ag}); err != nil {
				t.Fatalf("insert: %v", err)
			}
			if err := c.eng.DeleteRow(ctx, root, "fd_it_data", c.table, RowValues{"id": &annID}); err != nil {
				t.Fatalf("delete: %v", err)
			}
			nullFilter := BrowseRequest{Table: c.table, Filters: []Filter{{Column: "age", Op: OpIsNull}}}
			if res, err := c.eng.BrowseRows(ctx, root, "fd_it_data", nullFilter); err != nil {
				t.Errorf("null filter: %v", err)
			} else if res.Total != 0 {
				t.Errorf("after delete, rows with NULL age = %d, want 0", res.Total)
			}

			// Keyless tables are browse-only.
			one := "1"
			if err := c.eng.DeleteRow(ctx, root, "fd_it_data", nokey, RowValues{"a": &one}); err == nil {
				t.Error("delete on a keyless table should be refused")
			}
		})
	}
}

// TestIntegrationStructure exercises typed DDL on every configured engine.
func TestIntegrationStructure(t *testing.T) {
	cases := []struct {
		env, user, maint, parent, child string
		eng                             interface {
			Admin
			Structure
			Client
		}
	}{
		{"FLEETDOCK_IT_MARIADB", "root", "mysql", "authors", "books", &MariaDB{}},
		{"FLEETDOCK_IT_MYSQL", "root", "mysql", "authors", "books", &MariaDB{}},
		{"FLEETDOCK_IT_POSTGRES", "postgres", "postgres", "public.authors", "public.books", &Postgres{}},
	}
	for _, c := range cases {
		t.Run(c.env, func(t *testing.T) {
			ctx := context.Background()
			root := itParams(t, c.env, c.user)
			waitReady(t, c.eng, root)
			mustQuery(t, c.eng, root, c.maint, "DROP DATABASE IF EXISTS fd_it_ddl", true)
			mustQuery(t, c.eng, root, c.maint, "CREATE DATABASE fd_it_ddl", true)
			const db = "fd_it_ddl"
			isPG := c.user == "postgres"
			intType, textType := "int", "varchar(100)"
			if isPG {
				intType, textType = "integer", "varchar(100)"
			}
			quoteDef := "it's \\ fine"

			must := func(what string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("%s: %v", what, err)
				}
			}
			must("create authors", c.eng.CreateTable(ctx, root, db, TableSpec{
				Name: "authors",
				Columns: []ColumnSpec{
					{Name: "id", Type: intType, AutoIncrement: true},
					{Name: "name", Type: textType, Default: &quoteDef},
					{Name: "created_at", Type: "timestamp", Nullable: true, Default: ptrStr("CURRENT_TIMESTAMP"), DefaultIsExpression: true},
				},
				PrimaryKey: []string{"id"},
			}))
			must("create books", c.eng.CreateTable(ctx, root, db, TableSpec{
				Name: "books",
				Columns: []ColumnSpec{
					{Name: "id", Type: intType, AutoIncrement: true},
					{Name: "author_id", Type: intType},
					{Name: "title", Type: textType},
				},
				PrimaryKey:  []string{"id"},
				ForeignKeys: []ForeignKeySpec{{Name: "fk_books_author", Columns: []string{"author_id"}, RefTable: "authors", RefColumns: []string{"id"}, OnDelete: "cascade"}},
			}))
			// Default with a quote and a backslash round-trips.
			mustQuery(t, c.eng, root, db, "INSERT INTO authors (created_at) VALUES (NULL)", true)
			res := mustQuery(t, c.eng, root, db, "SELECT name FROM authors", false)
			if res.Rows[0][0] == nil || *res.Rows[0][0] != quoteDef {
				t.Errorf("default = %v, want %q", res.Rows[0][0], quoteDef)
			}

			fks, err := c.eng.ForeignKeys(ctx, root, db, c.child)
			must("foreign keys", err)
			if len(fks) != 1 || fks[0].Name != "fk_books_author" || fks[0].OnDelete != "CASCADE" || fks[0].RefColumns[0] != "id" {
				t.Errorf("foreign keys = %+v", fks)
			}

			must("alter", c.eng.AlterTable(ctx, root, db, c.child, []AlterOp{
				{Op: AlterAddColumn, Column: &ColumnSpec{Name: "pages", Type: intType, Nullable: true}},
				{Op: AlterModifyColumn, Name: "title", Column: &ColumnSpec{Name: "title", Type: "varchar(200)", Nullable: true}},
				{Op: AlterDropForeignKey, Name: "fk_books_author"},
			}))
			must("rename column", c.eng.AlterTable(ctx, root, db, c.child, []AlterOp{
				{Op: AlterRenameColumn, Name: "pages", NewName: "page_count"},
			}))
			// A failing op leaves nothing half-applied on PostgreSQL (and
			// is a single statement on MySQL).
			if err := c.eng.AlterTable(ctx, root, db, c.child, []AlterOp{
				{Op: AlterAddColumn, Column: &ColumnSpec{Name: "isbn", Type: textType, Nullable: true}},
				{Op: AlterDropColumn, Name: "does_not_exist"},
			}); err == nil {
				t.Error("alter with a bad op should fail")
			}
			mustQuery(t, c.eng, root, db, "SELECT id, author_id, title, page_count FROM books", false)
			if _, err := c.eng.Query(ctx, root, db, "SELECT isbn FROM books", 1, false); err == nil {
				t.Error("isbn column exists although its ALTER failed")
			}

			// Changing a column's type keeps its default and auto-increment.
			must("modify keep default", c.eng.AlterTable(ctx, root, db, c.parent, []AlterOp{
				{Op: AlterModifyColumn, Name: "name", Column: &ColumnSpec{Name: "name", Type: "varchar(150)", KeepDefault: true}},
				{Op: AlterModifyColumn, Name: "id", Column: &ColumnSpec{Name: "id", Type: "bigint", KeepDefault: true}},
			}))
			mustQuery(t, c.eng, root, db, "INSERT INTO authors (created_at) VALUES (NULL)", true)
			res = mustQuery(t, c.eng, root, db, "SELECT name FROM authors WHERE id = 2", false)
			if len(res.Rows) != 1 || res.Rows[0][0] == nil || *res.Rows[0][0] != quoteDef {
				t.Errorf("after modify, default/auto-increment lost: %v", res.Rows)
			}

			must("index", c.eng.CreateIndex(ctx, root, db, c.child, IndexSpec{Name: "ix_books_title", Columns: []string{"title"}}))
			must("drop index", c.eng.DropIndex(ctx, root, db, c.child, "ix_books_title"))
			must("rename", c.eng.RenameTable(ctx, root, db, c.child, "volumes"))
			renamed := "volumes"
			if isPG {
				renamed = "public.volumes"
			}
			must("truncate", c.eng.TruncateTable(ctx, root, db, renamed))

			// Rejected inputs never reach the server.
			if err := c.eng.CreateTable(ctx, root, db, TableSpec{Name: "x", Columns: []ColumnSpec{{Name: "a", Type: "int; DROP TABLE authors"}}}); err == nil {
				t.Error("injected type accepted")
			}
			if err := c.eng.CreateTable(ctx, root, db, TableSpec{Name: "x", Columns: []ColumnSpec{{Name: "a", Type: intType, Default: ptrStr("1); DROP TABLE authors; --"), DefaultIsExpression: true}}}); err == nil {
				t.Error("injected default expression accepted")
			}
			if err := c.eng.CreateTable(ctx, root, db, TableSpec{Name: "bad`name", Columns: []ColumnSpec{{Name: "a", Type: intType}}}); err == nil {
				t.Error("bad table name accepted")
			}

			mustQuery(t, c.eng, root, db, "CREATE VIEW author_names AS SELECT name FROM authors", true)
			objs, err := c.eng.Objects(ctx, root, db)
			must("objects", err)
			found := false
			for _, o := range objs {
				if o.Kind == ObjectView && o.Name == "author_names" && o.Definition != nil {
					found = true
				}
			}
			if !found {
				t.Errorf("view not listed: %+v", objs)
			}
			schema, err := c.eng.TableSchema(ctx, root, db, c.parent)
			must("schema", err)
			if !strings.Contains(schema.DDL, "PRIMARY KEY") {
				t.Errorf("DDL lacks the primary key:\n%s", schema.DDL)
			}
			must("drop", c.eng.DropTable(ctx, root, db, renamed))
		})
	}
}

func ptrStr(s string) *string { return &s }

// TestIntegrationQueryBatch checks session continuity and stop-at-first-error.
func TestIntegrationQueryBatch(t *testing.T) {
	cases := []struct {
		env, user, db string
		eng           interface {
			Admin
			Client
		}
		setVar, getVar, create string
	}{
		{"FLEETDOCK_IT_MARIADB", "root", "mysql", &MariaDB{}, "SET @fd = 41 + 1", "SELECT @fd", "CREATE TEMPORARY TABLE fd_tmp (a int)"},
		{"FLEETDOCK_IT_MYSQL", "root", "mysql", &MariaDB{}, "SET @fd = 41 + 1", "SELECT @fd", "CREATE TEMPORARY TABLE fd_tmp (a int)"},
		{"FLEETDOCK_IT_POSTGRES", "postgres", "postgres", &Postgres{}, "SET application_name = 'fd42'", "SHOW application_name", "CREATE TEMPORARY TABLE fd_tmp (a int)"},
	}
	for _, c := range cases {
		t.Run(c.env, func(t *testing.T) {
			ctx := context.Background()
			root := itParams(t, c.env, c.user)
			waitReady(t, c.eng, root)
			script := c.setVar + "; " + c.getVar + "; " + c.create + "; INSERT INTO fd_tmp VALUES (1); " +
				"BEGIN; INSERT INTO fd_tmp VALUES (2); ROLLBACK; SELECT count(*) FROM fd_tmp"
			stmts := SplitStatements(script)
			var pid int64
			res, err := c.eng.QueryBatch(ctx, root, c.db, stmts, 10, true, func(id int64) { pid = id })
			if err != nil {
				t.Fatalf("batch: %v", err)
			}
			if pid <= 0 {
				t.Error("session id not reported")
			}
			if v := res[1].Rows[0][0]; v == nil || !strings.Contains(*v, "42") {
				t.Errorf("session variable lost between statements: %v", v)
			}
			if v := res[len(res)-1].Rows[0][0]; v == nil || *v != "1" {
				t.Errorf("count after rollback = %v, want 1", v)
			}

			res, err = c.eng.QueryBatch(ctx, root, c.db, []string{"SELECT 1", "SELECT * FROM fd_no_such_table", "SELECT 3"}, 10, false, nil)
			var be *BatchError
			if !errors.As(err, &be) || be.Index != 1 || len(res) != 1 {
				t.Errorf("err=%v results=%d, want failure at statement 2 with 1 result", err, len(res))
			}
			if _, err := c.eng.QueryBatch(ctx, root, c.db, []string{"SELECT 1", "DELETE FROM x"}, 10, false, nil); err == nil {
				t.Error("write in a read-only batch must be refused up front")
			}
		})
	}
}

// TestIntegrationPostgresTextValues checks that browse, console and export show
// PostgreSQL values the way psql does, not as decoded Go values (a numeric
// printed as "{1337 -2 false finite true}", a uuid as a byte array).
func TestIntegrationPostgresTextValues(t *testing.T) {
	ctx := context.Background()
	pg := &Postgres{}
	root := itParams(t, "FLEETDOCK_IT_POSTGRES", "postgres")
	waitReady(t, pg, root)
	mustQuery(t, pg, root, "postgres", "DROP DATABASE IF EXISTS fd_it_text", true)
	mustQuery(t, pg, root, "postgres", "CREATE DATABASE fd_it_text", true)
	mustQuery(t, pg, root, "fd_it_text", `CREATE TABLE typed (id int primary key, amount numeric(12,2), day date,
		doc jsonb, uid uuid, ok boolean, blank text, missing text);
		INSERT INTO typed VALUES (1, 13.37, '2026-01-02', '{"a": [1, 2]}',
		'2c5fe2c6-6ad7-4662-a10b-cf2712877418', true, '', NULL)`, true)
	want := []*string{ptr("1"), ptr("13.37"), ptr("2026-01-02"), ptr(`{"a": [1, 2]}`),
		ptr("2c5fe2c6-6ad7-4662-a10b-cf2712877418"), ptr("t"), ptr(""), nil}
	check := func(what string, row []*string) {
		t.Helper()
		if len(row) != len(want) {
			t.Fatalf("%s: %d cells, want %d", what, len(row), len(want))
		}
		for i := range want {
			if (row[i] == nil) != (want[i] == nil) || (row[i] != nil && *row[i] != *want[i]) {
				t.Errorf("%s: cell %d = %v, want %v", what, i, deref(row[i]), deref(want[i]))
			}
		}
	}

	res, err := pg.BrowseRows(ctx, root, "fd_it_text", BrowseRequest{Table: "public.typed"})
	if err != nil || len(res.Rows) != 1 {
		t.Fatalf("browse: %v (%d rows)", err, len(res.Rows))
	}
	check("browse", res.Rows[0])
	q := mustQuery(t, pg, root, "fd_it_text", "SELECT * FROM typed", false)
	check("console", q.Rows[0])

	var buf strings.Builder
	if _, err := pg.ExportCSV(ctx, root, "fd_it_text", "public.typed", "", &buf, nil); err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(buf.String(), "1,13.37,2026-01-02,") || !strings.Contains(buf.String(), "2c5fe2c6-6ad7-4662-a10b-cf2712877418,t,,") {
		t.Errorf("export = %q", buf.String())
	}
}

func ptr(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
