//go:build integration

package executor

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// TestIntegrationProvisionClickHouse creates a ClickHouse container the way
// the agent does, checks the generated admin login can manage accounts, then
// stops, starts and removes it (volume included). Needs Docker and
// FLEETDOCK_IT_DOCKER_PORT, a free host port for the container.
func TestIntegrationProvisionClickHouse(t *testing.T) {
	ps := os.Getenv("FLEETDOCK_IT_DOCKER_PORT")
	if ps == "" {
		t.Skip("FLEETDOCK_IT_DOCKER_PORT not set")
	}
	port, _ := strconv.Atoi(ps)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	spec := &ProvisionSpec{
		ContainerName: "fleetdock-it-provision-ch",
		Image:         "clickhouse",
		Version:       "25.8",
		Port:          port,
		RootPassword:  "Pw-it-provision-1",
		Volume:        "fleetdock-it-provision-ch",
		RemoveVolume:  true,
	}
	p := &Payload{Engine: "clickhouse", Provision: spec}
	t.Cleanup(func() { _, _ = Execute(context.Background(), "remove_instance", p, NopSink{}) })

	out, err := Execute(ctx, "provision_instance", p, NopSink{})
	if err != nil {
		t.Fatal(err)
	}
	var res Result
	_ = json.Unmarshal(out, &res)
	if res.ContainerID == "" {
		t.Fatalf("no container id in %s", out)
	}

	ch, _ := engine.For("clickhouse")
	conn := engine.ConnParams{Host: "127.0.0.1", Port: port, User: "default", Password: spec.RootPassword, TLSMode: engine.TLSDisable}
	// Provisioning returned only once the server answered, so no retries.
	if _, err := ch.Ping(ctx, conn); err != nil {
		t.Fatalf("ping right after provisioning: %v", err)
	}
	admin, _ := engine.AdminFor("clickhouse")
	if err := admin.CreateDBUser(ctx, conn, "fd_it_user", "%", "Pw-user-1"); err != nil {
		t.Fatalf("the admin login must manage accounts with SQL: %v", err)
	}
	if err := ch.CreateDatabase(ctx, conn, "kept", "", ""); err != nil {
		t.Fatal(err)
	}

	for _, op := range []string{"stop_instance", "start_instance"} {
		if _, err := Execute(ctx, op, p, NopSink{}); err != nil {
			t.Fatalf("%s: %v", op, err)
		}
	}
	// Data survives a restart (it lives in the volume).
	deadline := time.Now().Add(60 * time.Second)
	for {
		dbs, err := ch.ListDatabases(ctx, conn)
		if err == nil {
			found := false
			for _, d := range dbs {
				found = found || d.Name == "kept"
			}
			if !found {
				t.Fatal("database lost across stop/start")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not back after start: %v", err)
		}
		time.Sleep(time.Second)
	}

	if _, err := Execute(ctx, "remove_instance", p, NopSink{}); err != nil {
		t.Fatal(err)
	}
	if _, err := runDocker(ctx, "volume", "inspect", spec.Volume); err == nil {
		t.Fatal("the volume should have been removed")
	}
}
