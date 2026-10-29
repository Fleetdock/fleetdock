package dbadminapp

import (
	"reflect"
	"testing"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

func sp(s string) *string { return &s }

func page(key []string, cols []string, rows ...engine.RowValues) *engine.ScanPage {
	return &engine.ScanPage{Key: key, Columns: cols, Rows: rows}
}

func TestCommonColumns(t *testing.T) {
	common, warnings := commonColumns([]string{"id", "name", "added"}, []string{"id", "old", "name"})
	if !reflect.DeepEqual(common, []string{"id", "name"}) || len(warnings) != 2 {
		t.Fatalf("common %v warnings %v", common, warnings)
	}
}

func TestDiffPages(t *testing.T) {
	key, cols := []string{"id"}, []string{"id", "name", "note"}
	live := page(key, cols,
		engine.RowValues{"id": sp("1"), "name": sp("ann"), "note": nil},     // unchanged
		engine.RowValues{"id": sp("2"), "name": sp("bob"), "note": sp("x")}, // changed note
		engine.RowValues{"id": sp("3"), "name": sp("new"), "note": nil},     // added since
	)
	backupHits := page(key, cols,
		engine.RowValues{"id": sp("1"), "name": sp("ann"), "note": nil},
		engine.RowValues{"id": sp("2"), "name": sp("bob"), "note": nil},
	)
	items := diffPages(live, backupHits, cols, true)
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Kind != "changed" || !reflect.DeepEqual(items[0].Changed, []string{"note"}) || items[0].LiveHash == "" {
		t.Errorf("changed = %+v", items[0])
	}
	if items[1].Kind != "added" || *items[1].Key["id"] != "3" || items[1].Backup != nil {
		t.Errorf("added = %+v", items[1])
	}
	if items[0].LiveHash != engine.RowHash(live.Rows[1], cols) {
		t.Error("live hash must fingerprint the live row")
	}

	// Second phase: rows of the backup gone from the live table.
	backup := page(key, cols,
		engine.RowValues{"id": sp("1"), "name": sp("ann"), "note": nil},
		engine.RowValues{"id": sp("9"), "name": sp("gone"), "note": nil},
	)
	liveHits := page(key, cols, engine.RowValues{"id": sp("1"), "name": sp("ann"), "note": nil})
	items = diffPages(backup, liveHits, cols, false)
	if len(items) != 1 || items[0].Kind != "removed" || *items[0].Key["id"] != "9" || items[0].Live != nil {
		t.Fatalf("removed = %+v", items)
	}
}

func TestCutLongValues(t *testing.T) {
	long := make([]byte, diffCellBytes+10)
	for i := range long {
		long[i] = 'a'
	}
	got := cut(engine.RowValues{"v": sp(string(long)), "n": nil}, []string{"v", "n"})
	if len(*got[0]) != diffCellBytes+len("…") || got[1] != nil {
		t.Fatalf("cut = %d bytes", len(*got[0]))
	}
}
