//go:build integration

// Package gateway integration tests drive a real HAProxy process: generated
// config must actually parse, the master socket must accept a reload, and the
// stats socket must report what the control plane reads back.
//
// Run with: go test -tags integration ./internal/platform/gateway/
package gateway

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// startHAProxy runs a real master-worker HAProxy against cfg and returns the
// socket paths. It skips (rather than fails) when no haproxy binary is present,
// so the suite stays runnable on a developer machine without one.
func startHAProxy(t *testing.T, cfg string) (configPath, masterSock, adminSock string) {
	t.Helper()

	bin, err := exec.LookPath("haproxy")
	if err != nil {
		t.Skip("haproxy binary not found in PATH")
	}

	dir := t.TempDir()
	configPath = filepath.Join(dir, "haproxy.cfg")
	masterSock = filepath.Join(dir, "m.sock")
	adminSock = filepath.Join(dir, "a.sock")

	if err := os.WriteFile(configPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(bin, "-W", "-db", "-f", configPath, "-S", masterSock)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start haproxy: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	waitForSocket(t, masterSock)
	return configPath, masterSock, adminSock
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("socket %s never appeared", path)
}

// baseConfig is what the gateway container is seeded with before any endpoint
// exists. It must be valid on its own.
func baseConfig(adminSock string) string {
	return Generate(nil, Options{AdminSocket: adminSock, HealthPort: 18404})
}

// TestGeneratedConfigLoadsInRealHAProxy is the check that would have caught an
// invalid directive reaching the live config.
func TestGeneratedConfigLoadsInRealHAProxy(t *testing.T) {
	adminSock := filepath.Join(t.TempDir(), "a.sock")
	maxConn := 25

	cfg := Generate([]Route{
		{ID: "a", ListenPort: 18432, BackendHost: "127.0.0.1", BackendPort: 15432,
			AllowedCIDRs: []string{"10.0.0.0/8"}, MaxConn: &maxConn},
		{ID: "b", ListenPort: 18433, BackendHost: "127.0.0.1", BackendPort: 15433,
			AllowedCIDRs: []string{"0.0.0.0/0"}},
		{ID: "c", ListenPort: 18434, BackendHost: "127.0.0.1", BackendPort: 15434},
	}, Options{AdminSocket: adminSock, HealthPort: 18404, DiagPort: 18431})

	startHAProxy(t, cfg)
	// Reaching here means HAProxy parsed and bound the generated config.
}

func TestApplyReloadsRunningHAProxy(t *testing.T) {
	adminSock := filepath.Join(t.TempDir(), "a.sock")
	configPath, masterSock, _ := startHAProxy(t, baseConfig(adminSock))

	r := NewReloader(Config{ConfigPath: configPath, MasterSocket: masterSock})

	updated := Generate([]Route{
		{ID: "a", ListenPort: 18432, BackendHost: "127.0.0.1", BackendPort: 15432, AllowedCIDRs: []string{"10.0.0.0/8"}},
	}, Options{AdminSocket: adminSock, HealthPort: 18404})

	changed, err := r.Apply(updated)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !changed {
		t.Fatal("a new config must report a change")
	}

	// The reconcile loop runs every minute; an unchanged config must not reload.
	changed, err = r.Apply(updated)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if changed {
		t.Fatal("identical config must be a no-op")
	}
}

// A config HAProxy rejects must leave the file matching what is still running,
// or the next Apply sees matching bytes and never retries the reload.
func TestApplyRollsBackRejectedConfig(t *testing.T) {
	adminSock := filepath.Join(t.TempDir(), "a.sock")
	good := baseConfig(adminSock)
	configPath, masterSock, _ := startHAProxy(t, good)

	r := NewReloader(Config{ConfigPath: configPath, MasterSocket: masterSock})

	if _, err := r.Apply(good + "\nfrontend broken\n  bind *:18999\n  not-a-directive yes\n"); err == nil {
		t.Fatal("expected HAProxy to reject the config")
	}

	onDisk, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(onDisk) != good {
		t.Fatalf("config must roll back to the version HAProxy is serving:\n%s", onDisk)
	}
}

func TestShowStatReportsGeneratedProxies(t *testing.T) {
	adminSock := filepath.Join(t.TempDir(), "a.sock")
	cfg := Generate([]Route{
		{ID: "a", ListenPort: 18432, BackendHost: "127.0.0.1", BackendPort: 15432, AllowedCIDRs: []string{"10.0.0.0/8"}},
	}, Options{AdminSocket: adminSock, HealthPort: 18404})

	startHAProxy(t, cfg)
	waitForSocket(t, adminSock)

	stats, err := ShowStat(adminSock)
	if err != nil {
		t.Fatalf("show stat: %v", err)
	}
	if _, ok := stats.Frontends[FrontendName(18432)]; !ok {
		t.Fatalf("frontend missing from stats: %v", stats.Frontends)
	}
	if _, ok := stats.Servers[BackendName(18432)]; !ok {
		t.Fatalf("backend server missing from stats: %v", stats.Servers)
	}
}
