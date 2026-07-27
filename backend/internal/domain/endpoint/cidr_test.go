package endpoint

import (
	"testing"
)

func TestValidateCIDR(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"host cidr", "89.12.24.10/32", "89.12.24.10/32"},
		{"network", "10.0.0.0/8", "10.0.0.0/8"},
		{"ipv4 anywhere", "0.0.0.0/0", AllowAnywhere},
		{"ipv6 anywhere", "::/0", AllowAnywhere},
		{"whitespace is trimmed", "  10.0.0.0/8  ", "10.0.0.0/8"},
		// A bare address is what people paste from "what is my IP".
		{"bare ipv4 becomes /32", "89.12.24.10", "89.12.24.10/32"},
		{"bare ipv6 becomes /128", "2001:db8::1", "2001:db8::1/128"},
		{"ipv6 network", "2001:db8::/32", "2001:db8::/32"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateCIDR(tc.in)
			if err != nil {
				t.Fatalf("%q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("%q => %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidateCIDRRejectsInvalid(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"garbage", "not-a-cidr"},
		{"empty", ""},
		{"whitespace only", "   "},
		{"bad mask", "10.0.0.0/64"},
		// Silently masking this to 10.1.2.0/24 would grant access to 255
		// addresses the author never named.
		{"host bits set", "10.1.2.3/24"},
		{"ipv6 host bits set", "2001:db8::1/32"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateCIDR(tc.in); err == nil {
				t.Fatalf("expected an error for %q", tc.in)
			}
		})
	}
}

func TestNormalizeCIDRs(t *testing.T) {
	t.Run("deduplicates", func(t *testing.T) {
		got, err := NormalizeCIDRs([]string{"10.0.0.0/8", "10.0.0.0/8", "89.12.24.10"})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"10.0.0.0/8", "89.12.24.10/32"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})

	// An empty allowlist must never reach the gateway: Generate treats it as
	// "reject everything", and the caller should be told to pick a network.
	t.Run("rejects an empty list", func(t *testing.T) {
		if _, err := NormalizeCIDRs(nil); err == nil {
			t.Fatal("expected an error for an empty allowlist")
		}
	})

	t.Run("propagates validation errors", func(t *testing.T) {
		if _, err := NormalizeCIDRs([]string{"10.0.0.0/8", "bogus"}); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestAllowsAnywhere(t *testing.T) {
	tests := []struct {
		name  string
		cidrs []string
		want  bool
	}{
		{"explicit ipv4", []string{"0.0.0.0/0"}, true},
		{"explicit ipv6", []string{"::/0"}, true},
		{"mixed with anywhere", []string{"10.0.0.0/8", "0.0.0.0/0"}, true},
		{"restricted", []string{"10.0.0.0/8"}, false},
		// Empty is not "anywhere" — Generate must fail closed instead.
		{"empty", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AllowsAnywhere(tc.cidrs); got != tc.want {
				t.Fatalf("AllowsAnywhere(%v) = %v, want %v", tc.cidrs, got, tc.want)
			}
		})
	}
}

func TestStatusTransitions(t *testing.T) {
	tests := []struct {
		from, to Status
		want     bool
	}{
		{StatusPending, StatusActive, true},
		{StatusPending, StatusError, true},
		{StatusActive, StatusError, true},
		{StatusActive, StatusDisabling, true},
		{StatusError, StatusActive, true},
		{StatusDisabling, StatusDisabled, true},
		{StatusDisabled, StatusPending, true},
		// A late reconcile must not resurrect a disabled endpoint.
		{StatusDisabled, StatusActive, false},
		{StatusDisabled, StatusError, false},
	}
	for _, tc := range tests {
		if got := tc.from.CanTransition(tc.to); got != tc.want {
			t.Errorf("%s -> %s = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestEndpointTarget(t *testing.T) {
	port := 15432
	public := &Endpoint{
		AccessType: AccessPublic, Protocol: ProtocolPostgreSQL, TLSMode: TLSRequired,
		ExternalHost: "gateway.example.com", ExternalPort: &port,
		InternalHost: "10.0.0.5", InternalPort: 5432,
	}
	got := public.Target()
	if got.Host != "gateway.example.com" || got.Port != 15432 {
		t.Fatalf("public target = %+v, want the external host and port together", got)
	}

	private := &Endpoint{
		AccessType: AccessPrivate, Protocol: ProtocolPostgreSQL, TLSMode: TLSPreferred,
		ExternalHost: "10.0.0.5", InternalHost: "10.0.0.5", InternalPort: 5432,
	}
	if got := private.Target(); got.Host != "10.0.0.5" || got.Port != 5432 {
		t.Fatalf("private target = %+v", got)
	}

	// A public endpoint with no allocated port must fall back to the internal
	// pair rather than mixing the public host with the private port.
	orphan := &Endpoint{
		AccessType: AccessPublic, Protocol: ProtocolPostgreSQL,
		ExternalHost: "gateway.example.com", InternalHost: "10.0.0.5", InternalPort: 5432,
	}
	if got := orphan.Target(); got.Host != "10.0.0.5" || got.Port != 5432 {
		t.Fatalf("orphan target = %+v", got)
	}
}
