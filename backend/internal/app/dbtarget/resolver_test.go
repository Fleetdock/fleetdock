package dbtarget

import (
	"context"
	"testing"

	"github.com/google/uuid"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/netsafe"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/sshtunnel"
)

type fakeServers struct {
	server *serverdom.Server
	err    error
}

func (f fakeServers) GetByID(context.Context, uuid.UUID) (*serverdom.Server, error) {
	return f.server, f.err
}

func ptr[T any](v T) *T { return &v }

func TestHostPrefersServerAddress(t *testing.T) {
	serverID := uuid.New()
	inst := &instancedom.Instance{Kind: instancedom.KindManaged, ServerID: &serverID}
	servers := fakeServers{server: &serverdom.Server{
		Hostname: "deluxe-damselfly",
		Address:  ptr("192.168.252.2/32"),
	}}

	host, err := Host(context.Background(), servers, inst, "instance_id")
	if err != nil {
		t.Fatalf("Host failed: %v", err)
	}
	// The agent reports its address in CIDR form; only the address is dialable.
	if host != "192.168.252.2" {
		t.Fatalf("host = %q, want 192.168.252.2", host)
	}
}

func TestHostExternalInstance(t *testing.T) {
	inst := &instancedom.Instance{Kind: instancedom.KindExternal, Host: ptr("db.example.com")}

	host, err := Host(context.Background(), fakeServers{}, inst, "instance")
	if err != nil {
		t.Fatalf("Host failed: %v", err)
	}
	if host != "db.example.com" {
		t.Fatalf("host = %q", host)
	}
}

func TestHostErrors(t *testing.T) {
	serverID := uuid.New()

	tests := []struct {
		name    string
		inst    *instancedom.Instance
		servers fakeServers
	}{
		{
			name:    "enrolled server has not reported an address",
			inst:    &instancedom.Instance{Kind: instancedom.KindManaged, ServerID: &serverID},
			servers: fakeServers{server: &serverdom.Server{Hostname: "deluxe-damselfly"}},
		},
		{
			name:    "server with neither address nor hostname",
			inst:    &instancedom.Instance{Kind: instancedom.KindManaged, ServerID: &serverID},
			servers: fakeServers{server: &serverdom.Server{}},
		},
		{
			name: "external instance with no host",
			inst: &instancedom.Instance{Kind: instancedom.KindExternal},
		},
		{
			name: "no server and no host",
			inst: &instancedom.Instance{Kind: instancedom.KindManaged},
		},
		{
			// An empty string used to pass through as a valid host and only
			// surfaced later as a confusing connection failure.
			name:    "empty address is not a host",
			inst:    &instancedom.Instance{Kind: instancedom.KindManaged, ServerID: &serverID},
			servers: fakeServers{server: &serverdom.Server{Hostname: "h", Address: ptr("")}},
		},
		{
			name: "external instance with an empty host",
			inst: &instancedom.Instance{Kind: instancedom.KindExternal, Host: ptr("")},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Host(context.Background(), tc.servers, tc.inst, "instance_id")
			if apperr.KindOf(err) != apperr.KindInvalid {
				t.Fatalf("expected an invalid error, got %v (%v)", apperr.KindOf(err), err)
			}
		})
	}
}

func TestHostUsesCallerField(t *testing.T) {
	inst := &instancedom.Instance{Kind: instancedom.KindExternal}

	_, err := Host(context.Background(), fakeServers{}, inst, "custom_field")
	var appErr *apperr.Error
	if !asAppErr(err, &appErr) {
		t.Fatalf("expected an apperr, got %T", err)
	}
	if appErr.Field != "custom_field" {
		t.Fatalf("field = %q, want custom_field", appErr.Field)
	}
}

func asAppErr(err error, target **apperr.Error) bool {
	e, ok := err.(*apperr.Error)
	if ok {
		*target = e
	}
	return ok
}

type fakeSecrets map[string][]byte

func (f fakeSecrets) Get(_ context.Context, ref string) ([]byte, error) { return f[ref], nil }

type fakePins struct{ keys []string }

func (f *fakePins) PinSSHHostKey(_ context.Context, _ uuid.UUID, key string) error {
	f.keys = append(f.keys, key)
	return nil
}

func TestHostBehindTunnelSkipsDBPolicy(t *testing.T) {
	netsafe.ConfigureDB(&netsafe.DBPolicy{})
	defer netsafe.ConfigureDB(nil)
	inst := &instancedom.Instance{Kind: instancedom.KindExternal, Host: ptr("127.0.0.1"), Port: 5432,
		SSH: &instancedom.SSHTunnel{Host: "10.0.0.5", Port: 22}}

	if host, err := Host(context.Background(), fakeServers{}, inst, "instance"); err != nil || host != "127.0.0.1" {
		t.Fatalf("Host = %q, %v; a host resolved on the bastion must not be checked locally", host, err)
	}
	inst.SSH = nil
	if _, err := Host(context.Background(), fakeServers{}, inst, "instance"); err == nil {
		t.Fatal("a direct loopback host must still be refused")
	}
}

func TestTunnel(t *testing.T) {
	if cfg, err := Tunnel(context.Background(), fakeSecrets{}, &fakePins{}, &instancedom.Instance{}, "f"); cfg != nil || err != nil {
		t.Fatalf("no tunnel: got %v, %v", cfg, err)
	}

	raw, _ := sshtunnel.Credentials{Password: "pw"}.Marshal()
	pins := &fakePins{}
	inst := &instancedom.Instance{ID: uuid.New(), Kind: instancedom.KindExternal, SSH: &instancedom.SSHTunnel{
		Host: "bastion", Port: 2222, User: "jump", Auth: instancedom.SSHAuthPassword, SecretRef: ptr("instance/x/ssh"),
	}}
	cfg, err := Tunnel(context.Background(), fakeSecrets{"instance/x/ssh": raw}, pins, inst, "f")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "bastion" || cfg.Port != 2222 || cfg.User != "jump" || cfg.Password != "pw" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if err := cfg.OnPin("ssh-ed25519 AAAA"); err != nil || len(pins.keys) != 1 {
		t.Fatalf("OnPin should persist the key: %v %v", err, pins.keys)
	}

	netsafe.ConfigureDB(&netsafe.DBPolicy{})
	defer netsafe.ConfigureDB(nil)
	inst.SSH.Host = "169.254.169.254"
	if _, err := Tunnel(context.Background(), fakeSecrets{"instance/x/ssh": raw}, pins, inst, "f"); err == nil {
		t.Fatal("a metadata bastion must be refused")
	}
}
