package engine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestParseCHError(t *testing.T) {
	e := parseCHError("Code: 81. DB::Exception: Database nope does not exist. (UNKNOWN_DATABASE) (version 25.8.33.6 (official build))\n")
	if e == nil || e.Code != 81 || e.Message != "Database nope does not exist. (UNKNOWN_DATABASE)" {
		t.Fatalf("parsed %+v", e)
	}
	if parseCHError("not an exception") != nil {
		t.Fatal("plain text is not an exception")
	}
	err := fmt.Errorf("statement 1: %w", &ClickHouseError{Code: chAccessDenied, Message: "x"})
	if !IsPermissionDenied(err) || IsLoginRejected(err) {
		t.Fatal("ACCESS_DENIED is a permission error, not a rejected login")
	}
	if !IsLoginRejected(&ClickHouseError{Code: chAuthenticationFailed}) {
		t.Fatal("AUTHENTICATION_FAILED is a rejected login")
	}
}

func TestCHCell(t *testing.T) {
	cases := map[string]*string{
		`null`:                 nil,
		`"NULL"`:               chPtr("NULL"),
		`"a\"b"`:               chPtr(`a"b`),
		`18446744073709551615`: chPtr("18446744073709551615"),
		`[1,"x"]`:              chPtr(`[1,"x"]`),
		`{"k":"v"}`:            chPtr(`{"k":"v"}`),
	}
	for in, want := range cases {
		got := chCell(json.RawMessage(in), 0)
		if (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Errorf("chCell(%s) = %v, want %v", in, chDeref(got), chDeref(want))
		}
	}
	if got := chCell(json.RawMessage(`"abcdef"`), 3); *got != "abc…" {
		t.Errorf("truncation = %q", *got)
	}
}

func TestCHParamsEscaping(t *testing.T) {
	v := `a\b	c`
	got := chParams([]any{`50\%`, &v, (*string)(nil)})
	if got["p1"] != `50\\%` || got["p2"] != `a\\b\tc` || got["p3"] != `\N` {
		t.Fatalf("params = %q", got)
	}
}

func TestClickHouseDialect(t *testing.T) {
	m := newTableMeta("`d`.`t`", []column{
		{Name: "id", Type: "UInt64"},
		{Name: "at", Type: "DateTime64(3, 'UTC')"},
	}, []string{"id"})
	one := "1"
	where, args, err := clickhouseDialect.where(m, BrowseRequest{
		Filters: []Filter{{Column: "id", Op: OpEq, Value: &one}, {Column: "at", Op: OpBetween, Values: []string{"a", "b"}}},
		Search:  "x",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := " WHERE `id` = {p1:UInt64} AND `at` BETWEEN {p2:DateTime64(3, 'UTC')} AND {p3:DateTime64(3, 'UTC')}" +
		" AND (toString(`id`) ILIKE concat('%', {p4:String}, '%') OR toString(`at`) ILIKE concat('%', {p4:String}, '%'))"
	if where != want {
		t.Fatalf("where =\n%s\nwant\n%s", where, want)
	}
	if len(args) != 4 {
		t.Fatalf("args = %v", args)
	}
}

func TestCHHostClause(t *testing.T) {
	for host, want := range map[string]string{"%": "HOST ANY", "": "HOST ANY", "localhost": "HOST LOCAL", "10.0.%": "HOST LIKE '10.0.%'"} {
		if got, err := chHostClause(host); err != nil || got != want {
			t.Errorf("chHostClause(%q) = %q, %v", host, got, err)
		}
	}
	if _, err := chHostClause("x' OR 1"); err == nil {
		t.Error("a host with quotes must be refused")
	}
}

func TestCHGrantList(t *testing.T) {
	got, err := chGrantList([]string{"SELECT", "delete", "ALL PRIVILEGES", "SELECT"})
	if err != nil || got != "SELECT, ALTER DELETE, ALL" {
		t.Fatalf("grant list = %q, %v", got, err)
	}
	if _, err := chGrantList([]string{"TRIGGER"}); err == nil {
		t.Fatal("TRIGGER has no ClickHouse equivalent")
	}
}

func TestCHQuoting(t *testing.T) {
	if got := quoteCHIdent("a`b\\c"); got != "`a\\`b\\\\c`" {
		t.Fatalf("ident = %s", got)
	}
	if got := quoteCHString(`it's \`); got != `'it\'s \\'` {
		t.Fatalf("string = %s", got)
	}
	if err := validCHUser("ok_user.1"); err != nil {
		t.Fatal(err)
	}
	if err := validCHUser("bad`user"); err == nil {
		t.Fatal("backtick in user name must be refused")
	}
}

func TestCHScheme(t *testing.T) {
	for _, tc := range []struct {
		mode string
		port int
		want string
	}{
		{"", 8123, "http"}, {"", 8443, "https"}, {TLSDisable, 8443, "http"},
		{TLSRequire, 8123, "https"}, {TLSVerifyFull, 8123, "https"},
	} {
		if got := chScheme(ConnParams{TLSMode: tc.mode, Port: tc.port}); got != tc.want {
			t.Errorf("mode %q port %d: %s, want %s", tc.mode, tc.port, got, tc.want)
		}
	}
}

func TestCHRegistered(t *testing.T) {
	for _, get := range []func(string) (any, error){
		func(n string) (any, error) { return AdminFor(n) },
		func(n string) (any, error) { return DataEditorFor(n) },
		func(n string) (any, error) { return MonitorFor(n) },
		func(n string) (any, error) { return CatalogFor(n) },
	} {
		if _, err := get("clickhouse"); err != nil {
			t.Fatal(err)
		}
	}
	if !IsSystemDatabase("clickhouse", "system") || IsSystemDatabase("clickhouse", "default") {
		t.Fatal("system is ClickHouse's only system database")
	}
	if !errors.Is(fmt.Errorf("wrap: %w", ErrReadOnlyEngine), ErrReadOnlyEngine) || !strings.Contains(ErrReadOnlyEngine.Error(), "SQL console") {
		t.Fatal("ErrReadOnlyEngine should point at the SQL console")
	}
}

func chPtr(s string) *string { return &s }

func chDeref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestRewriteCHDatabase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"CREATE TABLE src.t (a UInt8) ENGINE = Memory", "CREATE TABLE `dst`.t (a UInt8) ENGINE = Memory"},
		{"CREATE VIEW src.v AS SELECT * FROM `src`.t JOIN \"src\".u USING (id)", "CREATE VIEW `dst`.v AS SELECT * FROM `dst`.t JOIN `dst`.u USING (id)"},
		// Literals, comments, other qualifiers and look-alike names stay.
		{"SELECT 'src.t', x.src.y, srcx.t, a.src FROM src.t -- src.t", "SELECT 'src.t', x.src.y, srcx.t, a.src FROM `dst`.t -- src.t"},
		{"SOURCE(CLICKHOUSE(DB 'src' TABLE 'src'))", "SOURCE(CLICKHOUSE(DB 'dst' TABLE 'src'))"},
		// dictGet names the dictionary as a string; other strings stay.
		{"SELECT dictGet('src.kinds', 'n', k), dictGetOrDefault( 'src.kinds', 'n', k, 0), concat('src.kinds')", "SELECT dictGet('dst.kinds', 'n', k), dictGetOrDefault( 'dst.kinds', 'n', k, 0), concat('src.kinds')"},
	}
	for _, tc := range cases {
		if got := rewriteCHDatabase(tc.in, "src", "dst"); got != tc.want {
			t.Errorf("rewrite(%q)\n got %q\nwant %q", tc.in, got, tc.want)
		}
	}
}

func TestRewriteCHEngineDB(t *testing.T) {
	cases := []struct{ in, want string }{
		{"CREATE TABLE `dst`.b (x UInt8) ENGINE = Buffer('src', 'events', 16, 10, 100, 10000, 1000000, 10000000, 100000000)",
			"CREATE TABLE `dst`.b (x UInt8) ENGINE = Buffer('dst', 'events', 16, 10, 100, 10000, 1000000, 10000000, 100000000)"},
		{"CREATE TABLE `dst`.m (x UInt8) ENGINE = Merge(src, '^events')", "CREATE TABLE `dst`.m (x UInt8) ENGINE = Merge(`dst`, '^events')"},
		{"CREATE TABLE `dst`.d (x UInt8) ENGINE = Distributed('cluster', 'src', 'events', rand())",
			"CREATE TABLE `dst`.d (x UInt8) ENGINE = Distributed('cluster', 'dst', 'events', rand())"},
		// Another database, or a regexp, stays.
		{"CREATE TABLE `dst`.m (x UInt8) ENGINE = Merge(REGEXP('^s'), '^events')", "CREATE TABLE `dst`.m (x UInt8) ENGINE = Merge(REGEXP('^s'), '^events')"},
		{"CREATE TABLE `dst`.d (x UInt8) ENGINE = Distributed('src', 'other', 'events')", "CREATE TABLE `dst`.d (x UInt8) ENGINE = Distributed('src', 'other', 'events')"},
	}
	for _, tc := range cases {
		if got := rewriteCHEngineDB(tc.in, "src", "dst"); got != tc.want {
			t.Errorf("rewriteCHEngineDB(%q)\n got %q\nwant %q", tc.in, got, tc.want)
		}
	}
}

func TestCHRestoreDDLReplicated(t *testing.T) {
	in := "CREATE TABLE src.t (v UInt64) ENGINE = ReplicatedReplacingMergeTree('/clickhouse/tables/{shard}/src/t', '{replica}', v) ORDER BY v"
	got, notes := chRestoreDDL(in, "src", "copy", true, true)
	want := "CREATE TABLE `copy`.t (v UInt64) ENGINE = ReplacingMergeTree(v) ORDER BY v"
	if got != want || len(notes) != 1 {
		t.Fatalf("got %q (%v), want %q", got, notes, want)
	}
	if got, _ := chRestoreDDL("CREATE TABLE src.t (v UInt8) ENGINE = ReplicatedMergeTree ORDER BY v", "src", "copy", true, true); got != "CREATE TABLE `copy`.t (v UInt8) ENGINE = MergeTree() ORDER BY v" {
		t.Fatalf("bare Replicated engine: %q", got)
	}
	// ClickHouse Cloud's SharedMergeTree exists nowhere else.
	shared := "CREATE TABLE src.t (v UInt64) ENGINE = SharedReplacingMergeTree('/clickhouse/tables/{uuid}/{shard}', '{replica}', v) ORDER BY v"
	if got, _ := chRestoreDDL(shared, "src", "copy", true, true); got != want {
		t.Fatalf("Shared engine: %q", got)
	}
	// Restoring in place keeps the definition as it was.
	if got, notes := chRestoreDDL(in, "src", "src", false, false); got != in || notes != nil {
		t.Fatalf("in-place restore changed the DDL: %q", got)
	}
	// Under the same name on another server, replication is still dropped.
	if got, _ := chRestoreDDL(in, "src", "src", false, true); got != "CREATE TABLE src.t (v UInt64) ENGINE = ReplacingMergeTree(v) ORDER BY v" {
		t.Fatalf("same name, other server: %q", got)
	}
}

func TestCHSkipReason(t *testing.T) {
	kafka := chDumpRecord{Kind: "table", Name: "queue", Engine: "Kafka"}
	repl := chDumpRecord{Kind: "table", Name: "events", Engine: "ReplicatedMergeTree"}
	plain := chDumpRecord{Kind: "table", Name: "t", Engine: "MergeTree"}
	cases := []struct {
		rec                  chDumpRecord
		inPlace, deReplicate bool
		skip                 bool
	}{
		{kafka, false, true, true},  // a copy (new name, verification)
		{kafka, false, false, true}, // same name, unknown or other server
		{kafka, true, false, false}, // back where it came from
		{chDumpRecord{Kind: "table", Engine: "S3Queue"}, false, true, true},
		{repl, true, false, true},  // same name: would rejoin its replicas
		{repl, false, true, false}, // a copy: converted instead
		{plain, false, false, false},
	}
	for _, c := range cases {
		if got := chSkipReason(c.rec, c.inPlace, c.deReplicate, nil) != ""; got != c.skip {
			t.Errorf("%s inPlace=%v deReplicate=%v: skip = %v, want %v", c.rec.Engine, c.inPlace, c.deReplicate, got, c.skip)
		}
	}
}

func TestCHReadsFrom(t *testing.T) {
	mv := "CREATE MATERIALIZED VIEW `dst`.consumer TO `dst`.events AS SELECT _key, value FROM `dst`.queue"
	if got := chReadsFrom(mv, []string{"queue"}); got != "queue" {
		t.Errorf("a view reading the skipped queue: %q", got)
	}
	if got := chReadsFrom(mv, []string{"que", "events_queue"}); got != "" {
		t.Errorf("look-alike names matched: %q", got)
	}
}

func TestCHRenameCreate(t *testing.T) {
	cases := map[string]string{
		"CREATE TABLE `dst`.t (a UInt8) ENGINE = Memory":                     "CREATE TABLE `dst`.`.fd_restore.t` (a UInt8) ENGINE = Memory",
		"CREATE TABLE db.`odd name` (a UInt8) ENGINE = MergeTree ORDER BY a": "CREATE TABLE db.`.fd_restore.t` (a UInt8) ENGINE = MergeTree ORDER BY a",
	}
	for in, want := range cases {
		got, err := chRenameCreate(in, chRestoreTempName("t"))
		if err != nil || got != want {
			t.Errorf("chRenameCreate(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := chRenameCreate("CREATE VIEW db.v AS SELECT 1", "x"); err == nil {
		t.Error("only tables are renamed")
	}
}

func TestCHTrailingException(t *testing.T) {
	native := "\x02\x01\x00binary rows"
	if e := chTrailingException(native + "Code: 241. DB::Exception: Memory limit exceeded. (MEMORY_LIMIT_EXCEEDED)\n"); e == nil || e.Code != 241 {
		t.Errorf("current spelling not detected: %v", e)
	}
	if e := chTrailingException(native + "Code: 241, e.displayText() = DB::Exception: Memory limit exceeded\n"); e == nil || e.Code != 241 {
		t.Errorf("older spelling not detected: %v", e)
	}
	if e := chTrailingException(native); e != nil {
		t.Errorf("clean data reported as failed: %v", e)
	}
}

func TestCHChunkFraming(t *testing.T) {
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	payload := bytes.Repeat([]byte("x"), chDumpChunk+10)
	if err := writeCHRecord(bw, chDumpRecord{Type: "object", Name: "t"}, payload); err != nil {
		t.Fatal(err)
	}
	_ = bw.Flush()
	br := bufio.NewReader(&buf)
	if rec, err := readCHHeader(br); err != nil || rec.Name != "t" {
		t.Fatalf("header: %+v %v", rec, err)
	}
	got, err := io.ReadAll(&chChunkReader{r: br})
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("payload round trip: %d bytes, %v", len(got), err)
	}
	if _, err := io.ReadAll(&chChunkReader{r: bufio.NewReader(bytes.NewReader([]byte{0, 0, 1}))}); err == nil {
		t.Fatal("a cut chunk header must fail")
	}
}
