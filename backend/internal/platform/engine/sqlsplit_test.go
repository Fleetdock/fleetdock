package engine

import (
	"reflect"
	"testing"
)

func TestSplitStatements(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"single no semicolon", "SELECT 1", []string{"SELECT 1"}},
		{"two", "SELECT 1; SELECT 2;", []string{"SELECT 1", "SELECT 2"}},
		{"semicolon in string", "SELECT 'a;b'; SELECT 2", []string{"SELECT 'a;b'", "SELECT 2"}},
		{"doubled quote", "SELECT 'it''s; ok'; SELECT 3", []string{"SELECT 'it''s; ok'", "SELECT 3"}},
		{"backslash escape", `SELECT 'a\';b'; SELECT 4`, []string{`SELECT 'a\';b'`, "SELECT 4"}},
		{"quoted identifier", "SELECT \"a;b\" FROM `t;x`; SELECT 5", []string{"SELECT \"a;b\" FROM `t;x`", "SELECT 5"}},
		{"line comment", "SELECT 1; -- drop; this\nSELECT 2", []string{"SELECT 1", "-- drop; this\nSELECT 2"}},
		{"hash comment", "SELECT 1 # x;y\n; SELECT 2", []string{"SELECT 1 # x;y", "SELECT 2"}},
		{"block comment", "SELECT /* ; */ 1; SELECT 2", []string{"SELECT /* ; */ 1", "SELECT 2"}},
		{"comment only dropped", "SELECT 1; -- trailing note", []string{"SELECT 1"}},
		{"empty statements", ";; SELECT 1;;", []string{"SELECT 1"}},
		{
			"dollar quoted function",
			"CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql; SELECT f()",
			[]string{"CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql", "SELECT f()"},
		},
		{"tagged dollar", "SELECT $body$ a;b $body$; SELECT 2", []string{"SELECT $body$ a;b $body$", "SELECT 2"}},
		{"positional param is not a tag", "SELECT $1; SELECT 2", []string{"SELECT $1", "SELECT 2"}},
		{
			"mysql delimiter",
			"DELIMITER //\nCREATE PROCEDURE p() BEGIN SELECT 1; SELECT 2; END//\nDELIMITER ;\nCALL p();",
			[]string{"CREATE PROCEDURE p() BEGIN SELECT 1; SELECT 2; END", "CALL p()"},
		},
		{"unterminated string keeps text", "SELECT 'abc", []string{"SELECT 'abc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SplitStatements(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SplitStatements(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

func FuzzSplitStatements(f *testing.F) {
	for _, s := range []string{"SELECT 1;", "'a;b';$$x$$;", "DELIMITER //\nx//", "/* ; */--;\n#;"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, stmt := range SplitStatements(s) {
			if stmt == "" {
				t.Fatal("empty statement returned")
			}
		}
	})
}
