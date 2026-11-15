package instance

import (
	"testing"

	"github.com/google/uuid"
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
