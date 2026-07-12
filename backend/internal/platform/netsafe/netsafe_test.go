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
