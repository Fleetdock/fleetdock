package dbtarget

import (
	"context"
	"testing"

	"github.com/google/uuid"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
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
