package instance

import "testing"

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
