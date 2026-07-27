package conninfo

import (
	"strings"
	"testing"

	endpointdom "github.com/Fleetdock/fleetdock/backend/internal/domain/endpoint"
)

func pgTarget(mode endpointdom.TLSMode) endpointdom.Target {
	return endpointdom.Target{
		Host: "gateway.example.com", Port: 15432,
		Protocol: endpointdom.ProtocolPostgreSQL, TLSMode: mode,
	}
}

func TestBuildURLPercentEncodes(t *testing.T) {
	url, err := BuildURL(pgTarget(endpointdom.TLSRequired), "app user", "p@ss:word", "my db")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(url, "app%20user") {
		t.Fatalf("username not encoded: %s", url)
	}
	if !strings.Contains(url, "sslmode=require") {
		t.Fatalf("missing sslmode: %s", url)
	}
	if strings.Contains(url, "p@ss:word") {
		t.Fatalf("password not encoded: %s", url)
	}
}

func TestBuildURLTLSModes(t *testing.T) {
	tests := []struct {
		mode endpointdom.TLSMode
		want string
	}{
		{endpointdom.TLSRequired, "sslmode=require"},
		{endpointdom.TLSPreferred, "sslmode=prefer"},
		{endpointdom.TLSDisabled, "sslmode=disable"},
	}
	for _, tc := range tests {
		url, err := BuildURL(pgTarget(tc.mode), "u", "p", "db")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(url, tc.want) {
			t.Errorf("mode %q: want %q in %s", tc.mode, tc.want, url)
		}
	}
}

// mysqlsh is the only common client that understands ?ssl-mode= in a URL, so
// emitting it there just produces a string that most clients reject.
func TestBuildURLMySQLOmitsSSLModeQuery(t *testing.T) {
	for _, protocol := range []endpointdom.Protocol{endpointdom.ProtocolMySQL, endpointdom.ProtocolMariaDB} {
		target := endpointdom.Target{Host: "db.example.com", Port: 15433, Protocol: protocol, TLSMode: endpointdom.TLSRequired}
		url, err := BuildURL(target, "u", "p", "db")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(url, "ssl-mode") {
			t.Errorf("%s url must not carry ssl-mode: %s", protocol, url)
		}
		if !strings.HasPrefix(url, "mysql://") {
			t.Errorf("%s url should use the mysql scheme: %s", protocol, url)
		}
		// The mode still has to reach the user somewhere.
		if got := BuildFields(target, "u", "db").SSLMode; got != "REQUIRED" {
			t.Errorf("%s fields ssl_mode = %q, want REQUIRED", protocol, got)
		}
	}
}

// An unsupported engine used to fall through to a mysql:// URL, silently
// handing out a connection string for the wrong protocol.
func TestBuildURLRejectsUnknownProtocol(t *testing.T) {
	target := endpointdom.Target{Host: "h", Port: 1, Protocol: "", TLSMode: endpointdom.TLSRequired}
	if _, err := BuildURL(target, "u", "p", "db"); err == nil {
		t.Fatal("expected an error for an unknown protocol")
	}
}

func TestBuildURLValidation(t *testing.T) {
	tests := []struct {
		name   string
		target endpointdom.Target
	}{
		{"no host", endpointdom.Target{Port: 5432, Protocol: endpointdom.ProtocolPostgreSQL}},
		{"port zero", endpointdom.Target{Host: "h", Port: 0, Protocol: endpointdom.ProtocolPostgreSQL}},
		{"port too high", endpointdom.Target{Host: "h", Port: 70000, Protocol: endpointdom.ProtocolPostgreSQL}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildURL(tc.target, "u", "p", "db"); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// Fields must not repeat the password that the reveal payload already returns.
func TestBuildFieldsExcludesPassword(t *testing.T) {
	f := BuildFields(pgTarget(endpointdom.TLSRequired), "app", "mydb")
	if f.Host != "gateway.example.com" || f.Port != 15432 || f.User != "app" || f.Database != "mydb" {
		t.Fatalf("unexpected fields: %+v", f)
	}
	if f.SSLMode != "require" {
		t.Fatalf("ssl_mode = %q, want require", f.SSLMode)
	}
}

func TestCLICommand(t *testing.T) {
	tests := []struct {
		name     string
		target   endpointdom.Target
		contains []string
	}{
		{
			name:     "postgres",
			target:   pgTarget(endpointdom.TLSRequired),
			contains: []string{"psql", "postgresql://app@gateway.example.com:15432/mydb", "sslmode=require"},
		},
		{
			name:     "mariadb uses its own client",
			target:   endpointdom.Target{Host: "h", Port: 15433, Protocol: endpointdom.ProtocolMariaDB, TLSMode: endpointdom.TLSPreferred},
			contains: []string{"mariadb -h h -P 15433", "-u app", "--ssl-mode=PREFERRED", "mydb"},
		},
		{
			name:     "mysql",
			target:   endpointdom.Target{Host: "h", Port: 15434, Protocol: endpointdom.ProtocolMySQL, TLSMode: endpointdom.TLSRequired},
			contains: []string{"mysql -h h -P 15434", "--ssl-mode=REQUIRED"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CLICommand(tc.target, "app", "mydb")
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in %q", want, got)
				}
			}
		})
	}
}

func TestMaskPassword(t *testing.T) {
	masked := MaskPassword("postgresql://user:secret@gateway.example.com:15432/db")
	if strings.Contains(masked, "secret") {
		t.Fatalf("password not masked: %s", masked)
	}
}
