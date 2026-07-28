package engine

import (
	"context"
	"strings"
	"testing"
)

func TestIsSystemDatabase(t *testing.T) {
	cases := []struct {
		engine string
		name   string
		want   bool
	}{
		{"postgres", "postgres", true},
		{"postgres", "dev", false},
		{"postgres", "mysql", false},
		{"mysql", "mysql", true},
		{"mysql", "sys", true},
		{"mysql", "app", false},
		{"mariadb", "mysql", true},
		{"mariadb", "sys", true},
		{"mariadb", "postgres", false},
		{"", "postgres", false},
	}
	for _, c := range cases {
		if got := IsSystemDatabase(c.engine, c.name); got != c.want {
			t.Errorf("IsSystemDatabase(%q, %q) = %v, want %v", c.engine, c.name, got, c.want)
		}
	}
}

// DropDatabase must refuse system databases before it opens a connection, so
// the guard holds even for callers that bypass the application layer.
func TestDropDatabaseRefusesSystemDatabases(t *testing.T) {
	ctx := context.Background()
	p := ConnParams{Host: "127.0.0.1", Port: 1, User: "u", Password: "p"}

	for _, name := range []string{"mysql", "sys"} {
		err := (&MariaDB{}).DropDatabase(ctx, p, name)
		if err == nil || !strings.Contains(err.Error(), "system database") {
			t.Errorf("MariaDB.DropDatabase(%q) = %v, want refusal", name, err)
		}
	}
	err := (&Postgres{}).DropDatabase(ctx, p, "postgres")
	if err == nil || !strings.Contains(err.Error(), "system database") {
		t.Errorf("Postgres.DropDatabase(postgres) = %v, want refusal", err)
	}
}
