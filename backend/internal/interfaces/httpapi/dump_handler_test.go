package httpapi

import (
	"testing"
	"time"
)

func TestDumpFilename(t *testing.T) {
	at := time.Date(2026, 10, 12, 14, 30, 5, 0, time.UTC)
	cases := map[string]string{
		"orders":     "orders-20261012-143005.sql.gz",
		"my.db/../x": "my_db____x-20261012-143005.sql.gz",
		"":           "table-20261012-143005.sql.gz",
	}
	for in, want := range cases {
		if got := dumpFilename(in, at); got != want {
			t.Errorf("dumpFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
