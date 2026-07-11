package main

import "testing"

func TestParseRouteSrc(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "multipass style",
			output: "192.168.252.1 via 192.168.252.1 dev enp0s1 src 192.168.252.2 uid 1000",
			want:   "192.168.252.2",
		},
		{
			name:   "no src",
			output: "local 127.0.0.1 dev lo",
			want:   "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseRouteSrc(tc.output); got != tc.want {
				t.Fatalf("parseRouteSrc() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestControlPlaneHost(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"http://192.168.252.1:8080", "192.168.252.1"},
		{"https://dbm.example.com", "dbm.example.com"},
		{"not-a-url", ""},
	}
	for _, tc := range tests {
		if got := controlPlaneHost(tc.url); got != tc.want {
			t.Fatalf("controlPlaneHost(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestIsIPv4(t *testing.T) {
	if !isIPv4("192.168.1.1") {
		t.Fatal("expected ipv4")
	}
	if isIPv4("::1") {
		t.Fatal("expected not ipv4")
	}
}
