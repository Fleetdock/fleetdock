package engine

import (
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
)

func TestMariaDBDSNSpecialCharacters(t *testing.T) {
	p := ConnParams{Host: "db.example.com", Port: 3306, User: "root", Password: "p@ss/w?rd:#", Database: "app"}
	cfg, err := mysql.ParseDSN((&MariaDB{}).dsn(p))
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if cfg.Passwd != p.Password || cfg.User != "root" || cfg.DBName != "app" || cfg.Addr != "db.example.com:3306" {
		t.Errorf("round trip mismatch: %+v", cfg)
	}
	if cfg.Net != mysqlNet {
		t.Errorf("net = %q, want guarded %q", cfg.Net, mysqlNet)
	}
}

func TestTLSModes(t *testing.T) {
	cases := map[string]string{"": "preferred", TLSPrefer: "preferred", TLSDisable: "false", TLSRequire: "skip-verify", TLSVerifyFull: "true"}
	for mode, want := range cases {
		if got := mysqlTLS(mode); got != want {
			t.Errorf("mysqlTLS(%q) = %q, want %q", mode, got, want)
		}
		cs := (&Postgres{}).connString(ConnParams{Host: "h", Port: 5432, TLSMode: mode}, "")
		pgWant := mode
		if pgWant == "" {
			pgWant = TLSPrefer
		}
		if !strings.Contains(cs, "sslmode="+pgWant) {
			t.Errorf("pg conn string %q lacks sslmode=%s", cs, pgWant)
		}
		if _, err := pgx.ParseConfig(cs); err != nil {
			t.Errorf("pgx cannot parse %q: %v", cs, err)
		}
	}
}
