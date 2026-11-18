package instance

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

func TestTLSModeOrDefault(t *testing.T) {
	cases := []struct {
		engine Engine
		mode   string
		want   string
	}{
		{EnginePostgres, "", "prefer"},
		{EngineMariaDB, "", "prefer"},
		// ClickHouse cannot negotiate TLS in-band, so it must not default to
		// plain HTTP.
		{EngineClickHouse, "", "require"},
		{EngineClickHouse, "disable", "disable"},
		{EnginePostgres, "verify-full", "verify-full"},
	}
	for _, c := range cases {
		i := &Instance{Engine: c.engine, TLSMode: c.mode}
		if got := i.TLSModeOrDefault(); got != c.want {
			t.Errorf("%s with %q: got %q, want %q", c.engine, c.mode, got, c.want)
		}
	}
}

func TestFamilies(t *testing.T) {
	if EngineClickHouse.Family() != DumpFamily("clickhouse-native") || EngineClickHouse.Family() == EngineMySQL.Family() {
		t.Error("ClickHouse is its own family, matching its backups")
	}
	if EngineMariaDB.Family() != EngineMySQL.Family() || EnginePostgres.Family() != DumpFamily("pg_dump") {
		t.Error("existing families changed")
	}
}

func TestNewProvisionedClickHouseIsPlainHTTP(t *testing.T) {
	for _, port := range []int{8123, 8443} {
		inst, err := NewProvisioned(uuid.New(), "ch", EngineClickHouse, "25.8", port)
		if err != nil {
			t.Fatal(err)
		}
		if got := inst.TLSModeOrDefault(); got != "disable" {
			t.Errorf("port %d: tls = %q, want disable (the container serves plain HTTP)", port, got)
		}
	}
	pg, _ := NewProvisioned(uuid.New(), "pg", EnginePostgres, "17", 5432)
	if got := pg.TLSModeOrDefault(); got != "prefer" {
		t.Errorf("postgres tls = %q, want prefer", got)
	}
}

func TestValidateHost(t *testing.T) {
	for host, ok := range map[string]bool{
		"db.example.com":      true,
		"  db.example.com  ":  true,
		"db.example.com.":     true,
		"postgres":            true,
		"my_db-1":             true,
		"10.0.0.5":            true,
		"2001:db8::1":         true,
		"":                    false,
		"db.example.com\nfoo": false,
		"x\nfrontend pwn\n  bind *:15440\nbackend b\n  server s postgres:5432\n#": false,
		"db example.com":                 false,
		"nx.invalid,127.0.0.1":           false,
		"/var/run/postgresql":            false,
		"db:5432":                        false,
		"-db":                            false,
		"db..example.com":                false,
		"fe80::1%eth0":                   false,
		strings.Repeat("a", 64) + ".com": false,
	} {
		_, err := ValidateHost(host)
		if (err == nil) != ok {
			t.Errorf("ValidateHost(%q): err = %v, want ok=%v", host, err, ok)
		}
	}
}

func TestNewExternalRejectsInjectedHost(t *testing.T) {
	_, err := NewExternal("ext", EnginePostgres, "16", "x\nfrontend pwn", 5432, nil, nil, nil)
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("err = %v, want invalid", err)
	}
}
