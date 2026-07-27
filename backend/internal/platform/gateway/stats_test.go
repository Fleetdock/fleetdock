package gateway

import (
	"os"
	"strings"
	"testing"
)

// testdata/show_stat.csv is real output from haproxy 3.0.25, captured with one
// backend down and one connection rejected by the allowlist.
func TestParseStatsRealOutput(t *testing.T) {
	f, err := os.Open("testdata/show_stat.csv")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	stats, err := ParseStats(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	fe, ok := stats.Frontends[FrontendName(15432)]
	if !ok {
		t.Fatalf("missing frontend, got %v", stats.Frontends)
	}
	if fe.Status != "OPEN" {
		t.Errorf("frontend status = %q, want OPEN", fe.Status)
	}
	// The rejected connection is the signal that tells a user their allowlist
	// is turning away real clients.
	if fe.DeniedConn != 1 {
		t.Errorf("denied connections = %d, want 1", fe.DeniedConn)
	}

	srv, ok := stats.Servers[BackendName(15432)]
	if !ok {
		t.Fatalf("missing backend server, got %v", stats.Servers)
	}
	if srv.Status != "DOWN" {
		t.Errorf("server status = %q, want DOWN", srv.Status)
	}
	if srv.IsUp() {
		t.Error("a DOWN server must not report as up")
	}
}

func TestProxyStatIsUp(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"UP", true},
		{"UP 2/3", true}, // transitioning up, still serving
		{"OPEN", true},
		{"no check", true}, // health checks disabled
		{"DOWN", false},
		{"DOWN 1/2", false},
		{"MAINT", false},
		{"NOLB", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := (ProxyStat{Status: tc.status}).IsUp(); got != tc.want {
			t.Errorf("IsUp(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

// Columns are located by header name so a HAProxy upgrade that inserts or
// reorders fields does not silently shift every value.
func TestParseStatsUsesHeaderNames(t *testing.T) {
	csv := "# svname,status,pxname,dcon,stot\n" +
		"FRONTEND,OPEN,fe_15432,7,3\n"

	stats, err := ParseStats(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fe := stats.Frontends["fe_15432"]
	if fe.DeniedConn != 7 || fe.SessionsTotal != 3 || fe.Status != "OPEN" {
		t.Fatalf("header-driven parse failed: %+v", fe)
	}
}

func TestParseStatsErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"no header marker", "pxname,svname,status\nfe_1,FRONTEND,OPEN\n"},
		{"header without pxname", "# svname,status\nFRONTEND,OPEN\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseStats(strings.NewReader(tc.in)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
