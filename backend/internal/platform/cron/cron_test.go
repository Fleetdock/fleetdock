package cron

import (
	"testing"
	"time"
)

func TestNext(t *testing.T) {
	cases := []struct {
		expr string
		from string
		want string
	}{
		{"0 3 * * *", "2026-07-05T01:00:00Z", "2026-07-05T03:00:00Z"},    // daily 03:00
		{"0 3 * * *", "2026-07-05T04:00:00Z", "2026-07-06T03:00:00Z"},    // next day
		{"*/15 * * * *", "2026-07-05T10:07:00Z", "2026-07-05T10:15:00Z"}, // every 15 min
		{"0 * * * *", "2026-07-05T10:30:00Z", "2026-07-05T11:00:00Z"},    // hourly
		{"0 0 1 * *", "2026-07-05T00:00:00Z", "2026-08-01T00:00:00Z"},    // monthly 1st
		{"0 9 * * 1", "2026-07-05T00:00:00Z", "2026-07-06T09:00:00Z"},    // Mondays 09:00
	}
	for _, c := range cases {
		s, err := Parse(c.expr)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.expr, err)
		}
		from, _ := time.Parse(time.RFC3339, c.from)
		want, _ := time.Parse(time.RFC3339, c.want)
		if got := s.Next(from); !got.Equal(want) {
			t.Errorf("Next(%q, %s) = %s, want %s", c.expr, c.from, got.Format(time.RFC3339), c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, expr := range []string{"", "* * * *", "60 * * * *", "* 24 * * *", "a * * * *", "* * * * 8"} {
		if _, err := Parse(expr); err == nil {
			t.Errorf("Parse(%q) expected error, got nil", expr)
		}
	}
}
