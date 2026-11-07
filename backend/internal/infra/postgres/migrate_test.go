package postgres

import (
	"strings"
	"testing"
)

func TestEmbeddedMigrationsAreOrderedAndChecksummed(t *testing.T) {
	files, err := embeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 29 {
		t.Fatalf("only %d migrations embedded", len(files))
	}
	for i, m := range files {
		if len(m.checksum) != 64 {
			t.Errorf("%s: checksum %q", m.name, m.checksum)
		}
		if i > 0 && files[i-1].name >= m.name {
			t.Errorf("out of order: %s before %s", files[i-1].name, m.name)
		}
	}
}

func TestCheckHistory(t *testing.T) {
	files := []migration{{name: "0001_a.sql", checksum: "aaa"}, {name: "0002_b.sql", checksum: "bbb"}}

	for _, ok := range []map[string]string{
		{},
		{"0001_a.sql": "aaa"},
		{"0001_a.sql": "", "0002_b.sql": "bbb"}, // recorded before checksums
	} {
		if err := checkHistory(files, ok); err != nil {
			t.Errorf("%v: %v", ok, err)
		}
	}

	err := checkHistory(files, map[string]string{"0001_a.sql": "aaa", "0003_c.sql": "ccc"})
	if err == nil || !strings.Contains(err.Error(), "newer Fleetdock") || !strings.Contains(err.Error(), "0003_c.sql") {
		t.Errorf("unknown migration: %v", err)
	}
	err = checkHistory(files, map[string]string{"0001_a.sql": "zzz"})
	if err == nil || !strings.Contains(err.Error(), "0001_a.sql changed") {
		t.Errorf("changed migration: %v", err)
	}
}
