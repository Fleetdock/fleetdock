package instance

import "testing"

func TestCanRestoreInto(t *testing.T) {
	cases := []struct {
		src, dst Engine
		want     bool
	}{
		{EngineClickHouse, EngineClickHouse, true},
		{EngineClickHouse, EnginePostgres, false},
		{EnginePostgres, EngineClickHouse, false},
		{EngineMariaDB, EngineClickHouse, false},
		{EngineMariaDB, EngineMySQL, true},
		{EnginePostgres, EnginePostgres, true},
	}
	for _, tc := range cases {
		if got := tc.src.CanRestoreInto(tc.dst); got != tc.want {
			t.Errorf("%s into %s = %v, want %v", tc.src, tc.dst, got, tc.want)
		}
	}
	if EngineClickHouse.SupportsSQLImport() || EngineClickHouse.SupportsBackupCompare() || !EnginePostgres.SupportsSQLImport() {
		t.Error("ClickHouse lacks SQL import and backup compare; the others have them")
	}
}
