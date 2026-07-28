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
	stmts := postgresGrantStmts("mydb", "app_user", []string{"SELECT", "INSERT"}, []string{"public"})
	if len(stmts) < 2 {
		t.Fatalf("expected schema + table grants, got %v", stmts)
	}
	foundTable := false
	for _, s := range stmts {
		if containsAll(s, "GRANT", "SELECT", "INSERT", `ALL TABLES IN SCHEMA "public"`, "app_user") {
			foundTable = true
		}
	}
	if !foundTable {
		t.Fatalf("missing table grant statement: %v", stmts)
	}

	all := postgresGrantStmts("mydb", "app_user", []string{"ALL PRIVILEGES"}, []string{"public"})
	if len(all) < 3 {
		t.Fatalf("ALL PRIVILEGES should emit database, schema and table grants, got %v", all)
	}
}

// A PostgreSQL database can hold many schemas; a grant that only reaches public
// leaves the credential useless everywhere else.
func TestPostgresGrantStmtsCoversEverySchema(t *testing.T) {
	schemas := []string{"public", "sales", "audit"}
	stmts := postgresGrantStmts("mydb", "app_user", []string{"SELECT"}, schemas)

	for _, schema := range schemas {
		q := `"` + schema + `"`
		var usage, tables, future bool
		for _, s := range stmts {
			switch {
			case containsAll(s, "GRANT", "ON SCHEMA "+q, "app_user"):
				usage = true
			case containsAll(s, "GRANT", "ALL TABLES IN SCHEMA "+q, "app_user"):
				tables = true
			case containsAll(s, "ALTER DEFAULT PRIVILEGES IN SCHEMA "+q, "ON TABLES", "app_user"):
				future = true
			}
		}
		if !usage || !tables || !future {
			t.Errorf("schema %q: usage=%v tables=%v future=%v — want all true in %v",
				schema, usage, tables, future, stmts)
		}
	}
}

func TestPostgresGrantStmtsEmpty(t *testing.T) {
	if stmts := postgresGrantStmts("mydb", "app_user", nil, []string{"public"}); len(stmts) != 0 {
		t.Fatalf("expected no statements, got %v", stmts)
	}
	// No schemas (an empty database) still grants nothing at schema scope.
	if stmts := postgresGrantStmts("mydb", "app_user", []string{"SELECT"}, nil); len(stmts) != 0 {
		t.Fatalf("expected no statements without schemas, got %v", stmts)
	}
}

// Every profile the application layer can ask for must have a PostgreSQL
// mapping, at all four grant scopes.
func TestPGProfilesComplete(t *testing.T) {
	for _, p := range []AccessProfile{ProfileReadonly, ProfileReadWrite, ProfileAdmin} {
		g, ok := pgProfiles[p]
		if !ok {
			t.Fatalf("profile %q has no PostgreSQL mapping", p)
		}
		if g.database == "" || g.schema == "" || g.tables == "" || g.sequences == "" {
			t.Errorf("profile %q has an empty grant scope: %+v", p, g)
		}
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
