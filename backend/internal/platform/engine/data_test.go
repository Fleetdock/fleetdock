package engine

import (
	"strings"
	"testing"
)

func ptrS(s string) *string { return &s }

func testMeta(q func(string) string) *tableMeta {
	cols := []column{
		{Name: "id", Type: "integer"},
		{Name: "name", Type: "character varying(20)", Nullable: true, HasDefault: true},
		{Name: "email", Type: "text"},
	}
	m := newTableMeta(q("public")+"."+q("users"), cols, nil)
	m.key = chooseKey([][]string{{"id"}}, m.byName)
	return m
}

func TestBrowseWhereIsParameterised(t *testing.T) {
	m := testMeta(quotePGIdent)
	evil := `x'; DROP TABLE users; --`
	where, args, err := pgDialect.where(m, BrowseRequest{
		Filters: []Filter{{Column: "name", Op: OpEq, Value: ptrS(evil)}, {Column: "email", Op: OpIsNull}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(where, "DROP") {
		t.Fatalf("value leaked into SQL: %s", where)
	}
	want := ` WHERE "name" = ($1::text)::character varying(20) AND "email" IS NULL`
	if where != want {
		t.Errorf("where = %q\nwant    %q", where, want)
	}
	if len(args) != 1 || args[0] != evil {
		t.Errorf("args = %v", args)
	}
}

func TestBrowseRejectsUnknownColumnsAndOps(t *testing.T) {
	m := testMeta(quoteMySQLIdent)
	if _, _, err := mysqlDialect.where(m, BrowseRequest{Filters: []Filter{{Column: "id`; DROP", Op: OpEq, Value: ptrS("1")}}}, nil); err == nil {
		t.Error("unknown column accepted")
	}
	if _, _, err := mysqlDialect.where(m, BrowseRequest{Filters: []Filter{{Column: "id", Op: "; DROP", Value: ptrS("1")}}}, nil); err == nil {
		t.Error("unknown operator accepted")
	}
	if _, err := mysqlDialect.orderBy(m, []SortKey{{Column: "nope"}}); err == nil {
		t.Error("unknown sort column accepted")
	}
}

func TestBrowseOrderAlwaysEndsWithKey(t *testing.T) {
	m := testMeta(quoteMySQLIdent)
	got, err := mysqlDialect.orderBy(m, []SortKey{{Column: "name", Desc: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got != " ORDER BY `name` DESC, `id` ASC" {
		t.Errorf("order = %q", got)
	}
}

func TestSearchEscapesWildcards(t *testing.T) {
	m := testMeta(quoteMySQLIdent)
	_, args, err := mysqlDialect.where(m, BrowseRequest{Search: "50%_off"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if args[0] != `50\%\_off` {
		t.Errorf("search arg = %q, want wildcards escaped", args[0])
	}
	// One "?" per searched column.
	if len(args) != len(m.columns) {
		t.Errorf("mysql search args = %d, want one per column (%d)", len(args), len(m.columns))
	}
	_, pgArgs, _ := pgDialect.where(m, BrowseRequest{Search: "x"}, nil)
	if len(pgArgs) != 1 {
		t.Errorf("pg search args = %d, want 1 reused $1", len(pgArgs))
	}
}

func TestUpdateAndDeleteNeedTheExactKey(t *testing.T) {
	m := testMeta(quotePGIdent)
	q, args, err := pgDialect.updateSQL(m, RowValues{"id": ptrS("7")}, RowValues{"email": ptrS("a@b"), "name": nil})
	if err != nil {
		t.Fatal(err)
	}
	want := `UPDATE "public"."users" SET "name" = ($1::text)::character varying(20), "email" = ($2::text)::text WHERE "id" = ($3::text)::integer`
	if q != want {
		t.Errorf("update = %q\nwant     %q", q, want)
	}
	if v, ok := args[0].(*string); !ok || v != nil {
		t.Errorf("NULL value should be a nil *string, got %#v", args[0])
	}
	if args[2] != "7" {
		t.Errorf("key arg = %#v, want \"7\"", args[2])
	}
	if _, _, err := pgDialect.deleteSQL(m, RowValues{}); err == nil {
		t.Error("delete without key accepted")
	}
	if _, _, err := pgDialect.deleteSQL(m, RowValues{"name": ptrS("x")}); err == nil {
		t.Error("delete by a non-key column accepted")
	}
	if _, _, err := pgDialect.updateSQL(m, RowValues{"id": ptrS("1")}, RowValues{"bogus": ptrS("x")}); err == nil {
		t.Error("update of unknown column accepted")
	}
}

func TestTablesWithoutKeyAreReadOnly(t *testing.T) {
	cols := map[string]column{"a": {Name: "a", Nullable: true}, "b": {Name: "b"}}
	if k := chooseKey([][]string{{"a"}}, cols); k != nil {
		t.Errorf("nullable unique key chosen: %v", k)
	}
	if k := chooseKey([][]string{{"a"}, {"b"}}, cols); len(k) != 1 || k[0] != "b" {
		t.Errorf("key = %v, want the NOT NULL unique [b]", k)
	}
	m := newTableMeta("t", []column{{Name: "a", Nullable: true}}, nil)
	if _, _, err := mysqlDialect.deleteSQL(m, RowValues{"a": ptrS("1")}); err != ErrNoRowKey {
		t.Errorf("err = %v, want ErrNoRowKey", err)
	}
}

func TestExactlyOne(t *testing.T) {
	if exactlyOne(1) != nil || exactlyOne(0) == nil || exactlyOne(2) == nil {
		t.Error("exactlyOne contract broken")
	}
}

func TestCSVBatches(t *testing.T) {
	m := testMeta(quoteMySQLIdent)
	var got [][]*string
	n, err := csvBatches(m, strings.NewReader("\uFEFFid,name\n1,ann\n2,\n3,\\N\n"), ImportOptions{EmptyAsNull: true},
		func(cols []column, rows [][]*string) error {
			if len(cols) != 2 || cols[1].Name != "name" {
				t.Errorf("cols = %v", cols)
			}
			got = append(got, rows...)
			return nil
		})
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if *got[0][1] != "ann" || got[1][1] != nil || got[2][1] != nil {
		t.Errorf("rows = %v", got)
	}
	if _, err := csvBatches(m, strings.NewReader("id,nope\n1,2\n"), ImportOptions{}, nil); err == nil {
		t.Error("unknown CSV column accepted")
	}
	if _, err := csvBatches(m, strings.NewReader("id,name\n1\n"), ImportOptions{}, nil); err == nil {
		t.Error("ragged row accepted")
	}
}

func TestMySQLAlterRefusesSameStatementReferences(t *testing.T) {
	err := (&MariaDB{}).AlterTable(t.Context(), ConnParams{}, "db", "t", []AlterOp{
		{Op: AlterAddColumn, Column: &ColumnSpec{Name: "x", Type: "int", Nullable: true}},
		{Op: AlterRenameColumn, Name: "x", NewName: "y"},
	})
	if err == nil || !strings.Contains(err.Error(), "same change") {
		t.Fatalf("err = %v, want a same-change refusal before any connection", err)
	}
}

func TestStructureValidation(t *testing.T) {
	for _, ok := range []string{"varchar(20)", "INT UNSIGNED", "decimal(10,2)", "text"} {
		if _, err := checkType(mysqlTypeRe, ok); err != nil {
			t.Errorf("mysql type %q rejected: %v", ok, err)
		}
	}
	for _, ok := range []string{"character varying(20)", "timestamp with time zone", "integer[]", "numeric(10, 2)", "jsonb"} {
		if _, err := checkType(pgTypeRe, ok); err != nil {
			t.Errorf("pg type %q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"int; drop table x", "varchar(20) DEFAULT 1", "text) --"} {
		if _, err := checkType(pgTypeRe, bad); err == nil {
			t.Errorf("type %q accepted", bad)
		}
	}
	lit := "o'brien \\"
	d, err := renderDefault(ColumnSpec{Default: &lit}, pgLiteral)
	if err != nil || d != ` DEFAULT E'o''brien \\'` {
		t.Errorf("pg default = %q (%v)", d, err)
	}
	expr := "now()"
	if d, err := renderDefault(ColumnSpec{Default: &expr, DefaultIsExpression: true}, pgLiteral); err != nil || d != " DEFAULT now()" {
		t.Errorf("expression default = %q (%v)", d, err)
	}
	bad := "(SELECT password FROM users)"
	if _, err := renderDefault(ColumnSpec{Default: &bad, DefaultIsExpression: true}, pgLiteral); err == nil {
		t.Error("arbitrary default expression accepted")
	}
}
