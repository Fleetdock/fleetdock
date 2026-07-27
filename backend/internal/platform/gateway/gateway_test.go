package gateway

import (
	"strings"
	"testing"
)

func TestGenerateDeterministic(t *testing.T) {
	routes := []Route{
		{ID: "b", ListenPort: 15433, BackendHost: "10.0.0.2", BackendPort: 3306, AllowedCIDRs: []string{"10.0.0.0/8"}},
		{ID: "a", ListenPort: 15432, BackendHost: "10.0.0.1", BackendPort: 5432, AllowedCIDRs: []string{"89.12.24.10/32"}},
	}
	a := Generate(routes, Options{})
	b := Generate(routes, Options{})
	if a != b {
		t.Fatal("config not deterministic")
	}
	if !strings.Contains(a, "bind *:15432") || !strings.Contains(a, "bind *:15433") {
		t.Fatalf("missing listeners: %s", a)
	}
	if !strings.Contains(a, "tcp-request connection reject unless") {
		t.Fatalf("missing acl enforcement: %s", a)
	}
	if !strings.Contains(a, "fe_gateway_health") {
		t.Fatalf("missing health frontend: %s", a)
	}
}

func TestGenerateEmptyRoutesHasListener(t *testing.T) {
	cfg := Generate(nil, Options{})
	if !strings.Contains(cfg, "bind *:8404") {
		t.Fatalf("empty routes must keep a health listener: %s", cfg)
	}
}

func TestGenerateDedupesSamePort(t *testing.T) {
	cfg := Generate([]Route{
		{ID: "a", ListenPort: 15432, BackendHost: "10.0.0.1", BackendPort: 5432},
		{ID: "b", ListenPort: 15432, BackendHost: "10.0.0.2", BackendPort: 3306},
	}, Options{})
	if strings.Count(cfg, "frontend fe_15432") != 1 {
		t.Fatalf("expected one frontend for port 15432: %s", cfg)
	}
	if !strings.Contains(cfg, "server srv1 10.0.0.1:5432") {
		t.Fatalf("expected first route by id to win: %s", cfg)
	}
}

// maxconn is silently ignored by HAProxy as a bare backend directive
// ("has no frontend capability"), so it has to ride on the server line.
func TestGenerateMaxConnOnServerLine(t *testing.T) {
	limit := 25
	cfg := Generate([]Route{
		{ID: "a", ListenPort: 15432, BackendHost: "10.0.0.1", BackendPort: 5432,
			AllowedCIDRs: []string{"10.0.0.0/8"}, MaxConn: &limit},
	}, Options{})

	if !strings.Contains(cfg, "server srv1 10.0.0.1:5432 check maxconn 25") {
		t.Fatalf("maxconn must be a server argument: %s", cfg)
	}
	for _, line := range strings.Split(cfg, "\n") {
		if strings.TrimSpace(line) == "maxconn 25" {
			t.Fatalf("maxconn must not be a bare backend directive: %s", cfg)
		}
	}
}

func TestGenerateAllowlistModes(t *testing.T) {
	tests := []struct {
		name      string
		cidrs     []string
		wantLines []string
		denyLines []string
	}{
		{
			name:      "empty allowlist fails closed",
			cidrs:     nil,
			wantLines: []string{"  tcp-request connection reject\n"},
		},
		{
			name:      "explicit anywhere emits no acl",
			cidrs:     []string{"0.0.0.0/0"},
			denyLines: []string{"tcp-request connection reject"},
		},
		{
			name:  "multiple cidrs are or-ed",
			cidrs: []string{"10.0.0.0/8", "192.168.1.5/32"},
			wantLines: []string{
				"acl allow_15432_0 src 10.0.0.0/8",
				"acl allow_15432_1 src 192.168.1.5/32",
				"tcp-request connection reject unless allow_15432_0 or allow_15432_1",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Generate([]Route{
				{ID: "a", ListenPort: 15432, BackendHost: "10.0.0.1", BackendPort: 5432, AllowedCIDRs: tc.cidrs},
			}, Options{})
			for _, want := range tc.wantLines {
				if !strings.Contains(cfg, want) {
					t.Errorf("missing %q in:\n%s", want, cfg)
				}
			}
			for _, deny := range tc.denyLines {
				if strings.Contains(cfg, deny) {
					t.Errorf("unexpected %q in:\n%s", deny, cfg)
				}
			}
		})
	}
}

func TestGenerateOptions(t *testing.T) {
	routes := []Route{{ID: "a", ListenPort: 15432, BackendHost: "10.0.0.1", BackendPort: 5432, AllowedCIDRs: []string{"10.0.0.0/8"}}}

	t.Run("diag port disabled by default", func(t *testing.T) {
		if cfg := Generate(routes, Options{}); strings.Contains(cfg, "fe_gateway_whoami") {
			t.Fatalf("whoami must be opt-in: %s", cfg)
		}
	})

	t.Run("diag port emits whoami", func(t *testing.T) {
		cfg := Generate(routes, Options{DiagPort: 15431})
		if !strings.Contains(cfg, "frontend fe_gateway_whoami") || !strings.Contains(cfg, "bind *:15431") {
			t.Fatalf("missing whoami frontend: %s", cfg)
		}
		if !strings.Contains(cfg, "%[src]") {
			t.Fatalf("whoami must report the source address: %s", cfg)
		}
	})

	t.Run("admin socket emits stats socket", func(t *testing.T) {
		cfg := Generate(routes, Options{AdminSocket: "/run/admin.sock"})
		if !strings.Contains(cfg, "stats socket /run/admin.sock mode 660 level admin") {
			t.Fatalf("missing stats socket: %s", cfg)
		}
	})

	t.Run("proxy protocol only on public binds", func(t *testing.T) {
		cfg := Generate(routes, Options{SourceIPMode: SourceIPProxyProtocol, DiagPort: 15431})
		if !strings.Contains(cfg, "bind *:15432 accept-proxy") {
			t.Fatalf("public bind must accept proxy protocol: %s", cfg)
		}
		if strings.Contains(cfg, "bind *:8404 accept-proxy") || strings.Contains(cfg, "bind *:15431 accept-proxy") {
			t.Fatalf("health/whoami binds must not accept proxy protocol: %s", cfg)
		}
	})
}

// Transcripts captured from a live haproxy 3.0.25 master socket.
func TestParseReloadResponse(t *testing.T) {
	const successWithDownBackend = `Success=1
--
[NOTICE]   (1) : New worker (480) forked
[NOTICE]   (1) : Loading success.
[NOTICE]   (1) : haproxy version is 3.0.25-eb573a937
[WARNING]  (1) : Former worker (470) exited with code 0 (Exit)
[ALERT]    (1) : backend 'be_15433' has no server available!
`

	const failure = `[NOTICE]   (9) : Reloading HAProxy
[ALERT]    (9) : config : parsing [/tmp/live.cfg:8] : unknown keyword 'this-is-not-a-valid-directive' in 'frontend' section
[ALERT]    (9) : config : Fatal errors found in configuration.
[WARNING]  (9) : Loading failure!
Success=0
`

	const unknownCommand = `Unknown command: 'boguscmd', but maybe one of the following ones is a better match:
  reload                                  : achieve a soft-reload (-sf) of haproxy
`

	tests := []struct {
		name    string
		resp    string
		wantErr bool
	}{
		// A healthy reload still echoes ALERT lines for unrelated down
		// backends; treating those as failure would fail every reload
		// whenever any database happened to be offline.
		{"success with a down backend", successWithDownBackend, false},
		{"config parse failure", failure, true},
		{"unknown command", unknownCommand, true},
		{"legacy master without status line", "[ALERT] (1) : Fatal errors found in configuration.\n", true},
		{"empty response", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := parseReloadResponse(tc.resp)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q", tc.resp)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
