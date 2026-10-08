//go:build integration

package engine

import (
	"context"
	"net"
	"strconv"
	"testing"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/sshtunnel"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/sshtunnel/sshtunneltest"
)

// TestIntegrationPostgresThroughSSHTunnel reaches PostgreSQL through an
// in-process bastion, addressing the database by a name only the bastion can
// resolve ("localhost" is resolved there; the control plane must not look it
// up itself).
func TestIntegrationPostgresThroughSSHTunnel(t *testing.T) {
	p := itParams(t, "FLEETDOCK_IT_POSTGRES", "postgres")
	pg := &Postgres{}
	waitReady(t, pg, p)

	srv := sshtunneltest.Start(t, sshtunneltest.HostKey(t), "tunnel-pw", nil)
	host, ps, _ := net.SplitHostPort(srv.Addr)
	port, _ := strconv.Atoi(ps)
	var pinned []string
	p.SSH = &sshtunnel.Config{Host: host, Port: port, User: "jump",
		Credentials: sshtunnel.Credentials{Password: "tunnel-pw"},
		OnPin:       func(k string) error { pinned = append(pinned, k); return nil }}
	p.Host = "localhost"

	ctx := context.Background()
	version, err := pg.Ping(ctx, p)
	if err != nil {
		t.Fatalf("ping through tunnel: %v", err)
	}
	if version == "" {
		t.Fatal("empty version")
	}
	dbs, err := pg.ListDatabases(ctx, p)
	if err != nil || len(dbs) == 0 {
		t.Fatalf("list databases through tunnel: %v (%d)", err, len(dbs))
	}
	if len(pinned) != 1 {
		t.Fatalf("host key should be pinned exactly once, got %d", len(pinned))
	}

	// A different bastion key is refused once pinned.
	other := sshtunneltest.Start(t, sshtunneltest.HostKey(t), "tunnel-pw", nil)
	_, ops, _ := net.SplitHostPort(other.Addr)
	p.SSH.Port, _ = strconv.Atoi(ops)
	if _, err := pg.Ping(ctx, p); err == nil {
		t.Fatal("a changed bastion host key must be refused")
	}
}
