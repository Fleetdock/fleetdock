// Package uiproxy runs the bundled Next.js dashboard as a child process on
// loopback and exposes it as an http.Handler.
//
// The control plane is a single image and a single port: the Go API owns
// /v1, /agent and the handful of other API paths, and everything else is
// proxied here. That keeps deployments to one domain and one container, and
// means nothing about the deployment host is baked into the dashboard bundle.
//
// The dashboard is optional. A bare `api` binary and a `go run ./cmd/api` dev
// loop have no bundle, and must behave exactly as they did before it existed.
package uiproxy

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config describes where the dashboard bundle lives and how to run it.
type Config struct {
	// Dir is the Next.js standalone output root — the directory holding
	// server.js. Empty disables the dashboard entirely.
	Dir string
	// NodeBin is the node executable. Defaults to "node" on $PATH.
	NodeBin string
	// Host and Port are the loopback address the child binds. This is never
	// published; the Go process is the only client.
	Host string
	Port int
	// StartupTimeout bounds how long Start waits for the first successful
	// connection before giving up and letting the supervisor keep retrying.
	StartupTimeout time.Duration
}

const (
	defaultHost           = "127.0.0.1"
	defaultPort           = 3000
	defaultNodeBin        = "node"
	defaultStartupTimeout = 45 * time.Second

	// Restart backoff bounds for a child that keeps exiting.
	minRestartDelay = 1 * time.Second
	maxRestartDelay = 30 * time.Second
)

// New returns a Supervisor for the bundled dashboard, or (nil, nil) when there
// is nothing to run.
//
// A missing bundle is deliberately not an error. The dashboard is a convenience
// bundled into the container image; agents enrolling, backups running and the
// API answering must not depend on it. Callers treat a nil Supervisor as
// "API only" and keep the pre-existing 404 behaviour for unknown paths.
func New(cfg Config) (*Supervisor, error) {
	if strings.TrimSpace(cfg.Dir) == "" {
		return nil, nil
	}

	dir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("resolve FLEETDOCK_UI_DIR: %w", err)
	}
	entry := filepath.Join(dir, "server.js")
	if _, err := os.Stat(entry); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Warn("dashboard disabled: no server.js in FLEETDOCK_UI_DIR", "dir", dir)
			return nil, nil
		}
		return nil, fmt.Errorf("stat %s: %w", entry, err)
	}

	cfg.Dir = dir
	if cfg.NodeBin == "" {
		cfg.NodeBin = defaultNodeBin
	}
	if cfg.Host == "" {
		cfg.Host = defaultHost
	}
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	if cfg.StartupTimeout <= 0 {
		cfg.StartupTimeout = defaultStartupTimeout
	}

	s := &Supervisor{cfg: cfg, exited: make(chan struct{})}
	s.handler = s.newHandler()
	return s, nil
}
