package netsafe

import (
	"net"
	"testing"
)

func TestBlocked(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},       // loopback
		{"::1", true},             // loopback v6
		{"169.254.169.254", true}, // cloud metadata / link-local
		{"0.0.0.0", true},         // unspecified
		{"fd00:ec2::254", true},   // v6 metadata
		{"224.0.0.1", true},       // multicast
		{"8.8.8.8", false},        // public
		{"93.184.216.34", false},  // public
		{"10.0.0.5", false},       // RFC1918 — allowed (LAN targets are legitimate)
		{"192.168.1.10", false},   // RFC1918 — allowed
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			if got := blocked(net.ParseIP(tc.ip)); got != tc.want {
				t.Errorf("blocked(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
	if !blocked(nil) {
		t.Error("blocked(nil) = false, want true")
	}
}

func TestCheckDBHost(t *testing.T) {
	ctx := t.Context()
	ConfigureDB(&DBPolicy{Deny: []string{"10.9.9.9:5432"}})
	defer ConfigureDB(nil)

	cases := []struct {
		host string
		port int
		ok   bool
	}{
		{"127.0.0.1", 3306, false},
		{"::1", 5432, false},
		{"169.254.169.254", 80, false},
		{"0.0.0.0", 3306, false},
		{"10.9.9.9", 5432, false}, // the metadata DB
		{"10.9.9.9", 3306, true},  // same host, other port
		{"10.0.0.5", 3306, true},
		{"8.8.8.8", 5432, true},
	}
	for _, tc := range cases {
		err := CheckDBHost(ctx, tc.host, tc.port)
		if (err == nil) != tc.ok {
			t.Errorf("CheckDBHost(%s:%d) err = %v, want ok=%v", tc.host, tc.port, err, tc.ok)
		}
	}

	ConfigureDB(&DBPolicy{AllowLoopback: true})
	if err := CheckDBHost(ctx, "127.0.0.1", 3306); err != nil {
		t.Errorf("loopback should be allowed with AllowLoopback: %v", err)
	}
	if err := CheckDBHost(ctx, "169.254.169.254", 80); err == nil {
		t.Error("metadata must stay blocked even with AllowLoopback")
	}
}

func TestDialDBRefusesBlocked(t *testing.T) {
	ConfigureDB(&DBPolicy{})
	defer ConfigureDB(nil)
	if _, err := DialDB(t.Context(), "tcp", "127.0.0.1:1"); err == nil {
		t.Fatal("expected loopback dial to be refused")
	}
}
