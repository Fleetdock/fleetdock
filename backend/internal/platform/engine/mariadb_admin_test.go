package engine

import "testing"

func TestAdminForMariaDB(t *testing.T) {
	admin, err := AdminFor("mariadb")
	if err != nil {
		t.Fatalf("AdminFor(mariadb): %v", err)
	}
	if _, ok := admin.(*MariaDB); !ok {
		t.Fatalf("expected *MariaDB, got %T", admin)
	}
}

func TestAdminForMySQL(t *testing.T) {
	admin, err := AdminFor("mysql")
	if err != nil {
		t.Fatalf("AdminFor(mysql): %v", err)
	}
	if _, ok := admin.(*MariaDB); !ok {
		t.Fatalf("expected *MariaDB, got %T", admin)
	}
}

func TestIsReadOnlyStmt(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want bool
	}{
		{"select", "SELECT 1", true},
		{"select lowercase", "select * from t", true},
		{"leading whitespace", "   \n\t SELECT 1", true},
		{"show", "SHOW TABLES", true},
		{"describe", "DESCRIBE t", true},
		{"desc", "DESC t", true},
		{"explain", "EXPLAIN SELECT 1", true},
		{"with cte select", "WITH x AS (SELECT 1) SELECT * FROM x", true},
		{"leading paren", "(SELECT 1)", true},
		{"line comment then select", "-- a comment\nSELECT 1", true},
		{"hash comment then select", "# a comment\nSELECT 1", true},
		{"block comment then select", "/* c */ SELECT 1", true},

		{"insert", "INSERT INTO t VALUES (1)", false},
		{"update", "UPDATE t SET a = 1", false},
		{"delete", "DELETE FROM t", false},
		{"drop", "DROP TABLE t", false},
		{"truncate", "TRUNCATE t", false},
		{"create", "CREATE TABLE t (id int)", false},
		{"grant", "GRANT ALL ON db.* TO u", false},
		{"comment hiding write", "/* SELECT */ DELETE FROM t", false},
		{"empty", "", false},
		{"whitespace only", "   \n ", false},
		{"selection-prefixed identifier", "SELECTED FROM t", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isReadOnlyStmt(tc.sql); got != tc.want {
				t.Errorf("isReadOnlyStmt(%q) = %v, want %v", tc.sql, got, tc.want)
			}
		})
	}
}

func TestEscapeMySQLString(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "secret", "secret"},
		{"single quote doubled", "a'b", "a''b"},
		{"backslash doubled", `a\b`, `a\\b`},
		// The injection vector: a backslash followed by a quote. Backslashes
		// must be escaped first so the trailing quote cannot escape the closing
		// delimiter of the string literal.
		{"backslash then quote", `\'`, `\\''`},
		{"trailing backslash", `pw\`, `pw\\`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapeMySQLString(tc.in); got != tc.want {
				t.Errorf("escapeMySQLString(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidTableName(t *testing.T) {
	valid := []string{"users", "my_table", "Orders2024", "a b", "tbl-1"}
	for _, s := range valid {
		if !validTableName(s) {
			t.Errorf("validTableName(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "back`tick", "quote'", `dquote"`, "back\\slash", "new\nline"}
	for _, s := range invalid {
		if validTableName(s) {
			t.Errorf("validTableName(%q) = true, want false", s)
		}
	}
}
