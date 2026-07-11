package engine

import (
	"strings"
	"testing"
)

func TestAdminForPostgres(t *testing.T) {
	admin, err := AdminFor("postgres")
	if err != nil {
		t.Fatalf("AdminFor(postgres): %v", err)
	}
	if _, ok := admin.(*Postgres); !ok {
		t.Fatalf("expected *Postgres, got %T", admin)
	}
}

func TestValidPGRole(t *testing.T) {
	valid := []string{"app_user", "db_1", "role_2"}
	for _, s := range valid {
		if !validPGRole(s) {
			t.Errorf("validPGRole(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "bad name", "quote'", "db-1"}
	for _, s := range invalid {
		if validPGRole(s) {
			t.Errorf("validPGRole(%q) = true, want false", s)
		}
	}
}

func TestPostgresGrantStmts(t *testing.T) {
	stmts := postgresGrantStmts("mydb", "app_user", []string{"SELECT", "INSERT"})
	if len(stmts) < 2 {
		t.Fatalf("expected schema + table grants, got %v", stmts)
	}
	foundTable := false
	for _, s := range stmts {
		if containsAll(s, "GRANT", "SELECT", "INSERT", "ALL TABLES IN SCHEMA public", "app_user") {
			foundTable = true
		}
	}
	if !foundTable {
		t.Fatalf("missing table grant statement: %v", stmts)
	}

	all := postgresGrantStmts("mydb", "app_user", []string{"ALL PRIVILEGES"})
	if len(all) < 3 {
		t.Fatalf("ALL PRIVILEGES should emit database, schema and table grants, got %v", all)
	}
}

func TestPostgresGrantStmtsEmpty(t *testing.T) {
	if stmts := postgresGrantStmts("mydb", "app_user", nil); len(stmts) != 0 {
		t.Fatalf("expected no statements, got %v", stmts)
	}
}

func TestBuildPostgresDDL(t *testing.T) {
	ddl := buildPostgresDDL("public", "users", []ColumnInfo{
		{Name: "id", Type: "integer", Nullable: false, Key: "PRI"},
		{Name: "email", Type: "text", Nullable: false},
	})
	if !containsAll(ddl, "CREATE TABLE", `"public"`, `"users"`, `"id"`, "PRIMARY KEY", `"email"`, "NOT NULL") {
		t.Fatalf("unexpected ddl: %s", ddl)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
