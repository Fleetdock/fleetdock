package instance

import "testing"

// Restores, copies and moves need the same Family (backup.Restore and
// move.Start compare DumpFamily/Family); ClickHouse backups are not SQL dumps.
func TestFamilyRestoreRule(t *testing.T) {
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
		{EnginePostgres, EngineMariaDB, false},
	}
	for _, tc := range cases {
		if got := tc.src.Family() == tc.dst.Family(); got != tc.want {
			t.Errorf("%s into %s = %v, want %v", tc.src, tc.dst, got, tc.want)
		}
	}
	if DumpFamily("clickhouse-native") != EngineClickHouse.Family() {
		t.Error("a ClickHouse backup restores into ClickHouse")
	}
	if EngineClickHouse.SupportsSQLImport() || EngineClickHouse.SupportsBackupCompare() || !EnginePostgres.SupportsSQLImport() {
		t.Error("ClickHouse lacks SQL import and backup compare; the others have them")
	}
}
