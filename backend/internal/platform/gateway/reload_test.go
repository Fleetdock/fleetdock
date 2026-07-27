package gateway

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// fakeMaster stands in for the HAProxy master CLI, counting reload commands so
// tests can assert that an unchanged config does not trigger one.
type fakeMaster struct {
	reloads  atomic.Int32
	response string
	listener net.Listener
}

func newFakeMaster(t *testing.T, response string) *fakeMaster {
	t.Helper()
	// macOS caps unix socket paths near 104 bytes; t.TempDir() under
	// /var/folders can exceed that, so keep the filename short.
	sock := filepath.Join(t.TempDir(), "m.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	m := &fakeMaster{response: response, listener: ln}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				line, err := bufio.NewReader(conn).ReadString('\n')
				if err != nil {
					return
				}
				if line == "reload\n" {
					m.reloads.Add(1)
				}
				_, _ = conn.Write([]byte(m.response))
			}()
		}
	}()
	return m
}

func (m *fakeMaster) path() string { return m.listener.Addr().String() }

func newTestReloader(t *testing.T, m *fakeMaster) (*Reloader, string) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "haproxy.cfg")
	return NewReloader(Config{ConfigPath: cfgPath, MasterSocket: m.path()}), cfgPath
}

func TestApplyWritesAndReloads(t *testing.T) {
	m := newFakeMaster(t, "Success=1\n")
	r, cfgPath := newTestReloader(t, m)

	changed, err := r.Apply("config-v1\n")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !changed {
		t.Fatal("first apply must report a change")
	}
	if got, _ := os.ReadFile(cfgPath); string(got) != "config-v1\n" {
		t.Fatalf("config not written: %q", got)
	}
	if n := m.reloads.Load(); n != 1 {
		t.Fatalf("expected 1 reload, got %d", n)
	}
}

// The reconcile loop runs every minute. Reloading each time forks a new HAProxy
// worker and orphans live client connections, so identical content must be a no-op.
func TestApplyUnchangedIsNoOp(t *testing.T) {
	m := newFakeMaster(t, "Success=1\n")
	r, _ := newTestReloader(t, m)

	if _, err := r.Apply("same\n"); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	for i := 0; i < 5; i++ {
		changed, err := r.Apply("same\n")
		if err != nil {
			t.Fatalf("repeat apply: %v", err)
		}
		if changed {
			t.Fatal("unchanged config must not report a change")
		}
	}
	if n := m.reloads.Load(); n != 1 {
		t.Fatalf("expected exactly 1 reload across 6 applies, got %d", n)
	}
}

// A rejected reload must leave the file matching what HAProxy is still serving,
// otherwise the next Apply sees matching bytes and skips the reload forever.
func TestApplyRollsBackOnReloadFailure(t *testing.T) {
	m := newFakeMaster(t, "Success=1\n")
	r, cfgPath := newTestReloader(t, m)

	if _, err := r.Apply("good\n"); err != nil {
		t.Fatalf("seed apply: %v", err)
	}

	failing := newFakeMaster(t, "Success=0\n[ALERT] config : Fatal errors found in configuration.\n")
	broken := NewReloader(Config{ConfigPath: cfgPath, MasterSocket: failing.path()})
	if _, err := broken.Apply("bad\n"); err == nil {
		t.Fatal("expected reload failure to surface")
	}
	if got, _ := os.ReadFile(cfgPath); string(got) != "good\n" {
		t.Fatalf("config must roll back to the served version, got %q", got)
	}
}

func TestApplyRequiresConfiguration(t *testing.T) {
	if _, err := NewReloader(Config{}).Apply("x"); err == nil {
		t.Fatal("expected an error when no config path is set")
	}

	// An unset master socket used to make reload() a silent no-op, so configs
	// were written and never applied.
	r := NewReloader(Config{ConfigPath: filepath.Join(t.TempDir(), "haproxy.cfg")})
	if _, err := r.Apply("x"); err == nil {
		t.Fatal("expected an error when no master socket is set")
	}
}
