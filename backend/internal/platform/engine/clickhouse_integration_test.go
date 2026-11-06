//go:build integration

// Integration tests for the ClickHouse engine against a real server,
// reached over its HTTP interface:
//
//	FLEETDOCK_IT_CLICKHOUSE=127.0.0.1:18123  (user "default", password in FLEETDOCK_IT_PASSWORD)
//
// The server needs SQL-driven access management for the default user
// (CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1 in the official image).
package engine

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func chIT(t *testing.T) (*ClickHouse, ConnParams) {
	t.Helper()
	c := &ClickHouse{}
	p := itParams(t, "FLEETDOCK_IT_CLICKHOUSE", "default")
	waitReady(t, c, p)
	return c, p
}

// chFixture creates a fresh database with an events table.
func chFixture(t *testing.T, c *ClickHouse, p ConnParams, name string) {
	t.Helper()
	ctx := context.Background()
	_ = c.DropDatabase(ctx, p, name)
	if err := c.CreateDatabase(ctx, p, name, "", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.DropDatabase(context.Background(), p, name) })
	mustQuery(t, c, p, name, `CREATE TABLE events (
		id UInt64,
		kind LowCardinality(String),
		amount Nullable(Int64),
		note String,
		at DateTime64(3, 'UTC'),
		tags Array(String),
		INDEX kind_idx kind TYPE set(10) GRANULARITY 1
	) ENGINE = MergeTree ORDER BY (id, toDate(at))`, true)
	res := mustQuery(t, c, p, name, `INSERT INTO events VALUES
		(1, 'click', 10,   'plain',        '2026-01-01 10:00:00', ['a']),
		(2, 'view',  NULL, '50% off',      '2026-01-02 10:00:00', []),
		(3, 'click', 30,   'back\\slash',  '2026-01-03 10:00:00', ['b','c']),
		(4, 'buy',   99,   'NULL',         '2026-01-04 10:00:00', [])`, true)
	if res.RowsAffected != 4 {
		t.Fatalf("insert reported %d rows affected, want 4", res.RowsAffected)
	}
}

func TestIntegrationClickHouseClient(t *testing.T) {
	c, p := chIT(t)
	ctx := context.Background()
	if v, err := c.Ping(ctx, p); err != nil || v == "" {
		t.Fatalf("ping: %q %v", v, err)
	}
	chFixture(t, c, p, "fd_it_client")

	dbs, err := c.ListDatabases(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]DatabaseInfo{}
	for _, d := range dbs {
		found[d.Name] = d
	}
	if _, ok := found["fd_it_client"]; !ok {
		t.Fatalf("created database not listed: %+v", dbs)
	}
	if !found["system"].System {
		t.Fatal("system database should be flagged System")
	}
	if _, ok := found["information_schema"]; ok {
		t.Fatal("virtual information_schema should not be listed")
	}
	if err := c.DropDatabase(ctx, p, "system"); err == nil {
		t.Fatal("dropping the system database should be refused")
	}
	if n, err := c.CountTables(ctx, p, "fd_it_client"); err != nil || n != 1 {
		t.Fatalf("count tables: %d %v", n, err)
	}

	bad := p
	bad.Password = "wrong"
	_, err = c.Ping(ctx, bad)
	if !IsLoginRejected(err) {
		t.Fatalf("wrong password should be a rejected login, got %v", err)
	}
}

func TestIntegrationClickHouseConsole(t *testing.T) {
	c, p := chIT(t)
	ctx := context.Background()
	chFixture(t, c, p, "fd_it_console")

	res := mustQuery(t, c, p, "fd_it_console", "SELECT id, amount, note, tags FROM events ORDER BY id", false)
	if res.RowCount != 4 || strings.Join(res.Columns, ",") != "id,amount,note,tags" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Rows[1][1] != nil {
		t.Fatalf("NULL should be nil, got %q", *res.Rows[1][1])
	}
	if got := *res.Rows[3][2]; got != "NULL" {
		t.Fatalf("the string 'NULL' should stay a string, got %q", got)
	}
	if got := *res.Rows[2][3]; got != `["b","c"]` {
		t.Fatalf("array cell = %q", got)
	}

	// Limits truncate.
	res, err := c.Query(ctx, p, "fd_it_console", "SELECT number FROM numbers(1000)", 10, false)
	if err != nil || res.RowCount != 10 || !res.Truncated {
		t.Fatalf("truncation: %+v %v", res, err)
	}

	// A statement with its own FORMAT is shown as text.
	res = mustQuery(t, c, p, "fd_it_console", "SELECT id, kind FROM events ORDER BY id LIMIT 2 FORMAT CSV", false)
	if len(res.Columns) != 1 || res.RowCount != 2 || *res.Rows[0][0] != `1,"click"` {
		t.Fatalf("FORMAT CSV result: %+v", res)
	}

	// Writes need allowWrite, and a read-only run cannot lift readonly.
	mustFail(t, c, p, "fd_it_console", "INSERT INTO events (id) VALUES (9)", false)
	if _, err := c.Query(ctx, p, "fd_it_console", "SELECT 1 SETTINGS readonly = 0", 10, false); err == nil {
		t.Fatal("a read-only run must not be able to turn readonly off")
	}

	// A batch shares one session: SET carries over.
	rs, err := c.QueryBatch(ctx, p, "fd_it_console", []string{
		"SET max_threads = 3",
		"SELECT getSetting('max_threads')",
	}, 10, true, nil)
	if err != nil || len(rs) != 2 || *rs[1].Rows[0][0] != "3" {
		t.Fatalf("session batch: %+v %v", rs, err)
	}
	// The failing statement is reported by index.
	_, err = c.QueryBatch(ctx, p, "fd_it_console", []string{"SELECT 1", "SELECT * FROM missing_table"}, 10, false, nil)
	var be *BatchError
	if !errors.As(err, &be) || be.Index != 1 {
		t.Fatalf("batch error: %v", err)
	}

	// The server enforces the statement timeout.
	tp := p
	tp.StatementTimeout = time.Second
	start := time.Now()
	if _, err := c.Query(ctx, tp, "fd_it_console", "SELECT count() FROM numbers_mt(1e12)", 10, false); err == nil {
		t.Fatal("statement should have timed out")
	}
	if time.Since(start) > 15*time.Second {
		t.Fatal("timeout was not enforced server-side")
	}

	var buf bytes.Buffer
	n, err := c.ExportCSV(ctx, p, "fd_it_console", "", "SELECT id, amount FROM events ORDER BY id", &buf, nil)
	if err != nil || n != 4 {
		t.Fatalf("export: %d %v", n, err)
	}
	if !strings.HasPrefix(buf.String(), "id,amount\n1,10\n2,\n") {
		t.Fatalf("export CSV = %q", buf.String())
	}
}

func TestIntegrationClickHouseBrowse(t *testing.T) {
	c, p := chIT(t)
	ctx := context.Background()
	chFixture(t, c, p, "fd_it_browse")
	ids := func(r *BrowseResult) string {
		var out []string
		for _, row := range r.Rows {
			out = append(out, *row[0])
		}
		return strings.Join(out, ",")
	}
	v := func(s string) *string { return &s }
	cases := []struct {
		name string
		req  BrowseRequest
		want string
	}{
		{"all, stable order", BrowseRequest{}, "1,2,3,4"},
		{"eq on integer", BrowseRequest{Filters: []Filter{{Column: "id", Op: OpEq, Value: v("3")}}}, "3"},
		{"eq on LowCardinality", BrowseRequest{Filters: []Filter{{Column: "kind", Op: OpEq, Value: v("click")}}}, "1,3"},
		{"literal percent", BrowseRequest{Filters: []Filter{{Column: "note", Op: OpContains, Value: v("50%")}}}, "2"},
		{"literal backslash", BrowseRequest{Filters: []Filter{{Column: "note", Op: OpContains, Value: v(`k\s`)}}}, "3"},
		{"is null", BrowseRequest{Filters: []Filter{{Column: "amount", Op: OpIsNull}}}, "2"},
		{"gt on nullable", BrowseRequest{Filters: []Filter{{Column: "amount", Op: OpGt, Value: v("20")}}}, "3,4"},
		{"in", BrowseRequest{Filters: []Filter{{Column: "kind", Op: OpIn, Values: []string{"buy", "view"}}}}, "2,4"},
		{"between datetimes", BrowseRequest{Filters: []Filter{{Column: "at", Op: OpBetween,
			Values: []string{"2026-01-02 00:00:00", "2026-01-03 23:00:00"}}}}, "2,3"},
		{"search any column", BrowseRequest{Search: "CLICK"}, "1,3"},
		{"sort desc", BrowseRequest{Sort: []SortKey{{Column: "amount", Desc: true}}}, "4,3,1,2"},
		{"page", BrowseRequest{Limit: 2, Offset: 2}, "3,4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.Table = "events"
			res, err := c.BrowseRows(ctx, p, "fd_it_browse", tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(res); got != tc.want {
				t.Fatalf("ids = %s, want %s", got, tc.want)
			}
			if len(res.Key) != 0 {
				t.Fatalf("ClickHouse rows must not be editable, got key %v", res.Key)
			}
		})
	}
	res, err := c.BrowseRows(ctx, p, "fd_it_browse", BrowseRequest{Table: "events"})
	if err != nil || res.Total != 4 || len(res.Columns) != 6 {
		t.Fatalf("total/columns: %+v %v", res, err)
	}
	if _, err := c.BrowseRows(ctx, p, "fd_it_browse", BrowseRequest{Table: "events",
		Filters: []Filter{{Column: "nope", Op: OpEq, Value: v("1")}}}); err == nil {
		t.Fatal("an unknown column must be refused")
	}

	var buf bytes.Buffer
	n, err := c.ExportRowsCSV(ctx, p, "fd_it_browse", BrowseRequest{Table: "events", Search: "click"}, &buf, nil)
	if err != nil || n != 2 {
		t.Fatalf("export rows: %d %v", n, err)
	}

	if _, err := c.UpdateRow(ctx, p, "fd_it_browse", "events", RowValues{"id": v("1")}, RowValues{"note": v("x")}); !errors.Is(err, ErrReadOnlyEngine) {
		t.Fatalf("UpdateRow = %v, want ErrReadOnlyEngine", err)
	}
}

func TestIntegrationClickHouseIntrospection(t *testing.T) {
	c, p := chIT(t)
	ctx := context.Background()
	chFixture(t, c, p, "fd_it_meta")
	mustQuery(t, c, p, "fd_it_meta", "CREATE VIEW clicks AS SELECT * FROM events WHERE kind = 'click'", true)

	tables, err := c.ListTables(ctx, p, "fd_it_meta")
	if err != nil || len(tables) != 1 || tables[0].Name != "events" || tables[0].RowCount != 4 {
		t.Fatalf("list tables (views are objects, not tables): %+v %v", tables, err)
	}
	objs, err := c.Objects(ctx, p, "fd_it_meta")
	if err != nil || len(objs) != 1 || objs[0].Name != "clicks" || objs[0].Kind != ObjectView ||
		objs[0].Definition == nil || !strings.Contains(*objs[0].Definition, "CREATE VIEW") {
		t.Fatalf("objects: %+v %v", objs, err)
	}
	s, err := c.TableSchema(ctx, p, "fd_it_meta", "events")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Columns) != 6 || s.Columns[0].Key != "PRI" || !s.Columns[2].Nullable || !strings.Contains(s.DDL, "MergeTree") {
		t.Fatalf("schema: %+v", s)
	}
	if len(s.Indexes) != 2 || s.Indexes[0].Name != "PRIMARY" || s.Indexes[1].Name != "kind_idx" {
		t.Fatalf("indexes: %+v", s.Indexes)
	}
	page, err := c.TableRows(ctx, p, "fd_it_meta", "events", 2, 0)
	if err != nil || len(page.Rows) != 2 || page.Total != 4 {
		t.Fatalf("table rows: %+v %v", page, err)
	}
	cat, err := c.Columns(ctx, p, "fd_it_meta")
	if err != nil || len(cat.Tables) != 2 || cat.Tables[0].Kind != "view" || cat.Tables[1].Kind != "table" {
		t.Fatalf("catalog: %+v %v", cat, err)
	}
	if !cat.Tables[1].Columns[0].PrimaryKey {
		t.Fatal("id should be reported as primary key")
	}
}

func TestIntegrationClickHouseAccessRoles(t *testing.T) {
	c, p := chIT(t)
	ctx := context.Background()
	chFixture(t, c, p, "fd_it_roles")
	chFixture(t, c, p, "fd_it_other")

	for _, u := range []string{"fd_it_reader", "fd_it_writer"} {
		_ = c.DropDBUser(ctx, p, u, "%")
		if err := c.CreateDBUser(ctx, p, u, "%", "Pw-"+u); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.DropDBUser(context.Background(), p, u, "%") })
	}
	if err := ApplyConsoleProfile(ctx, c, p, "fd_it_reader", "%", "fd_it_roles", false); err != nil {
		t.Fatal(err)
	}
	if err := ApplyConsoleProfile(ctx, c, p, "fd_it_writer", "%", "fd_it_roles", true); err != nil {
		t.Fatal(err)
	}
	as := func(user string) ConnParams { q := p; q.User, q.Password = user, "Pw-"+user; return q }
	reader, writer := as("fd_it_reader"), as("fd_it_writer")

	mustQuery(t, c, reader, "fd_it_roles", "SELECT count() FROM events", false)
	// Even with allowWrite, the readonly constraint refuses the write.
	mustFail(t, c, reader, "fd_it_roles", "INSERT INTO events (id) VALUES (10)", true)
	_, err := c.Query(ctx, reader, "fd_it_other", "SELECT count() FROM events", 10, false)
	if !IsPermissionDenied(err) {
		t.Fatalf("reading another database should be permission denied, got %v", err)
	}

	mustQuery(t, c, writer, "fd_it_roles", "INSERT INTO events (id) VALUES (10)", true)
	mustQuery(t, c, writer, "fd_it_roles", "CREATE TABLE scratch (x UInt8) ENGINE = Memory", true)
	mustFail(t, c, writer, "fd_it_other", "INSERT INTO events (id) VALUES (10)", true)

	grants, err := c.SchemaGrants(ctx, p, "fd_it_roles")
	if err != nil {
		t.Fatal(err)
	}
	users := map[string]bool{}
	for _, g := range grants {
		users[g.User] = true
	}
	if !users["fd_it_reader"] || !users["fd_it_writer"] {
		t.Fatalf("schema grants: %+v", grants)
	}
	if lines, err := c.UserGrants(ctx, p, "fd_it_reader", "%"); err != nil || len(lines) == 0 {
		t.Fatalf("user grants: %v %v", lines, err)
	}
	if err := c.Revoke(ctx, p, "fd_it_writer", "%", "fd_it_roles"); err != nil {
		t.Fatal(err)
	}
	mustFail(t, c, writer, "fd_it_roles", "SELECT count() FROM events", false)

	if err := RotatePassword(ctx, c, p, "fd_it_reader", "%", "Pw-rotated"); err != nil {
		t.Fatal(err)
	}
	rotated := reader
	rotated.Password = "Pw-rotated"
	mustQuery(t, c, rotated, "fd_it_roles", "SELECT 1", false)

	if err := c.Grant(ctx, p, "fd_it_writer", "%", "fd_it_roles", []string{"SELECT", "TRIGGER"}); err == nil {
		t.Fatal("a privilege ClickHouse lacks must be refused")
	}
}

func TestIntegrationClickHouseMonitor(t *testing.T) {
	c, p := chIT(t)
	ctx := context.Background()
	chFixture(t, c, p, "fd_it_mon")

	ids := make(chan int64, 1)
	done := make(chan error, 1)
	go func() {
		_, err := c.QueryBatch(ctx, p, "fd_it_mon", []string{"SELECT count() FROM numbers_mt(1e13)"}, 10, false,
			func(id int64) { ids <- id })
		done <- err
	}()
	id := <-ids

	var seen bool
	for i := 0; i < 50 && !seen; i++ {
		procs, err := c.Processes(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		for _, pr := range procs {
			if pr.ID == id {
				seen = true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !seen {
		t.Fatal("the running console statement was not listed under its session id")
	}
	if err := c.KillProcess(ctx, p, id, false); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the killed statement should have failed")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("KillProcess did not stop the statement")
	}

	status, err := c.ServerStatus(ctx, p)
	if err != nil || len(status) < 3 || status[0].Name != "Version" {
		t.Fatalf("status: %v %v", status, err)
	}
	if vars, err := c.Variables(ctx, p); err != nil || len(vars) < 10 {
		t.Fatalf("variables: %d %v", len(vars), err)
	}
	stats, err := c.DatabaseStats(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	var size int64 = -1
	for _, s := range stats {
		if s.Name == "fd_it_mon" {
			size = s.SizeBytes
		}
	}
	if size <= 0 {
		t.Fatalf("fd_it_mon size = %d in %+v", size, stats)
	}
}
