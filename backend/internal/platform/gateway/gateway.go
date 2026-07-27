// Package gateway generates and reloads HAProxy configuration for public endpoints.
package gateway

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	endpointdom "github.com/Fleetdock/fleetdock/backend/internal/domain/endpoint"
)

// Source IP modes control how HAProxy determines a client's address.
const (
	// SourceIPDirect trusts the address on the TCP connection itself.
	SourceIPDirect = "direct"
	// SourceIPProxyProtocol reads the real client address from a PROXY
	// protocol header, for deployments behind an L4 load balancer.
	SourceIPProxyProtocol = "proxy-protocol"
)

// DefaultHealthPort is the container-internal port serving the health check.
const DefaultHealthPort = 8404

// Config holds gateway runtime settings.
type Config struct {
	ConfigPath   string
	MasterSocket string
}

// Options tunes the generated configuration.
type Options struct {
	// AdminSocket, when set, exposes a stats socket for runtime queries.
	AdminSocket string
	// HealthPort serves the always-present health frontend. Zero uses DefaultHealthPort.
	HealthPort int
	// DiagPort serves a plaintext "what source IP do you have" endpoint. Zero disables it.
	DiagPort int
	// SourceIPMode is SourceIPDirect or SourceIPProxyProtocol.
	SourceIPMode string
}

// Route is one public TCP endpoint rendered into HAProxy.
type Route struct {
	ID           string
	ListenPort   int
	BackendHost  string
	BackendPort  int
	AllowedCIDRs []string
	MaxConn      *int
}

// Generate builds a deterministic HAProxy configuration from routes.
func Generate(routes []Route, opts Options) string {
	sorted := append([]Route(nil), routes...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].ListenPort == sorted[j].ListenPort {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].ListenPort < sorted[j].ListenPort
	})

	healthPort := opts.HealthPort
	if healthPort == 0 {
		healthPort = DefaultHealthPort
	}

	var b strings.Builder
	b.WriteString("global\n")
	b.WriteString("  log stdout format raw local0\n")
	b.WriteString("  maxconn 50000\n")
	if opts.AdminSocket != "" {
		// Read by ShowStat to report per-endpoint backend health and the number
		// of connections the allowlist rejected.
		b.WriteString(fmt.Sprintf("  stats socket %s mode 660 level admin expose-fd listeners\n", opts.AdminSocket))
		b.WriteString("  stats timeout 30s\n")
	}
	b.WriteString("\n")
	b.WriteString("defaults\n")
	b.WriteString("  mode tcp\n")
	b.WriteString("  log global\n")
	b.WriteString("  option tcplog\n")
	b.WriteString("  timeout connect 10s\n")
	b.WriteString("  timeout client  1h\n")
	b.WriteString("  timeout server  1h\n")
	b.WriteString("\n")

	// HAProxy -c exits 2 when a config has no listeners. Always keep the health
	// frontend so empty public-route sets still validate and stay reloadable.
	b.WriteString("frontend fe_gateway_health\n")
	b.WriteString(fmt.Sprintf("  bind *:%d\n", healthPort))
	b.WriteString("  mode http\n")
	b.WriteString("  http-request return status 200\n")
	b.WriteString("\n")

	// Reports the source address HAProxy actually observes. Behind NAT or an L4
	// load balancer that is the proxy's address, not the client's — which is the
	// difference between a working allowlist and one that rejects everyone.
	if opts.DiagPort > 0 {
		b.WriteString("frontend fe_gateway_whoami\n")
		b.WriteString(fmt.Sprintf("  bind *:%d\n", opts.DiagPort))
		b.WriteString("  mode http\n")
		b.WriteString(`  http-request return status 200 content-type "text/plain" lf-string "%[src]\n"` + "\n")
		b.WriteString("\n")
	}

	bindSuffix := ""
	if opts.SourceIPMode == SourceIPProxyProtocol {
		bindSuffix = " accept-proxy"
	}

	seenPorts := make(map[int]struct{}, len(sorted))
	for _, r := range sorted {
		if _, dup := seenPorts[r.ListenPort]; dup {
			continue // one public listener per port
		}
		seenPorts[r.ListenPort] = struct{}{}

		frontend := fmt.Sprintf("fe_%d", r.ListenPort)
		backend := fmt.Sprintf("be_%d", r.ListenPort)
		b.WriteString(fmt.Sprintf("frontend %s\n", frontend))
		b.WriteString(fmt.Sprintf("  bind *:%d%s\n", r.ListenPort, bindSuffix))
		b.WriteString("  mode tcp\n")
		switch {
		case len(r.AllowedCIDRs) == 0:
			// Fail closed: an endpoint with no allowlist reaches nobody. The
			// column defaults to an empty array, so treating empty as
			// allow-all would silently expose a database.
			b.WriteString("  tcp-request connection reject\n")
		case endpointdom.AllowsAnywhere(r.AllowedCIDRs):
			// Explicitly open; no ACL needed.
		default:
			aclName := fmt.Sprintf("allow_%d", r.ListenPort)
			for i, cidr := range r.AllowedCIDRs {
				b.WriteString(fmt.Sprintf("  acl %s_%d src %s\n", aclName, i, cidr))
			}
			conds := make([]string, len(r.AllowedCIDRs))
			for i := range r.AllowedCIDRs {
				conds[i] = fmt.Sprintf("%s_%d", aclName, i)
			}
			b.WriteString(fmt.Sprintf("  tcp-request connection reject unless %s\n", strings.Join(conds, " or ")))
		}
		b.WriteString(fmt.Sprintf("  default_backend %s\n\n", backend))

		b.WriteString(fmt.Sprintf("backend %s\n", backend))
		b.WriteString("  mode tcp\n")
		// maxconn belongs on the server line: HAProxy ignores it as a bare
		// backend directive ("has no frontend capability").
		serverOpts := ""
		if r.MaxConn != nil && *r.MaxConn > 0 {
			serverOpts = fmt.Sprintf(" maxconn %d", *r.MaxConn)
		}
		b.WriteString(fmt.Sprintf("  server srv1 %s:%d check%s\n\n", r.BackendHost, r.BackendPort, serverOpts))
	}
	return b.String()
}

// Reloader writes, validates, and reloads HAProxy configuration.
type Reloader struct {
	cfg Config
}

// NewReloader builds a reloader.
func NewReloader(cfg Config) *Reloader {
	return &Reloader{cfg: cfg}
}

// Apply writes the config and triggers a hitless reload, reporting whether
// anything changed. An unchanged config is a no-op: the reconcile loop runs
// every minute, and reloading each time forks a new HAProxy worker and orphans
// long-lived client connections for no reason.
//
// Validation happens by way of the reload itself rather than a local `haproxy -c`.
// The API image would have to ship the same HAProxy version as the gateway for
// pre-flight validation to mean anything, and a rejected reload is harmless —
// the running workers keep serving the previous config.
func (r *Reloader) Apply(content string) (changed bool, err error) {
	if r.cfg.ConfigPath == "" {
		return false, fmt.Errorf("gateway config path is not configured")
	}
	dir := filepath.Dir(r.cfg.ConfigPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("create config dir: %w", err)
	}

	previous, readErr := os.ReadFile(r.cfg.ConfigPath)
	if readErr == nil && string(previous) == content {
		return false, nil
	}

	if err := r.write(content); err != nil {
		return false, err
	}
	if err := r.reload(); err != nil {
		// Put the previous config back so the file on disk keeps matching what
		// HAProxy is actually serving; otherwise the next Apply would see
		// matching bytes and skip the reload that never succeeded.
		if readErr == nil {
			if rbErr := r.write(string(previous)); rbErr != nil {
				return true, fmt.Errorf("%w (rollback also failed: %v)", err, rbErr)
			}
			if rbErr := r.reload(); rbErr != nil {
				return true, fmt.Errorf("%w (rollback reload also failed: %v)", err, rbErr)
			}
		}
		return false, err
	}
	return true, nil
}

// write atomically replaces the config file. The temp file lives beside the
// target so the rename stays within one filesystem.
func (r *Reloader) write(content string) error {
	tmp := r.cfg.ConfigPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, r.cfg.ConfigPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// reloadTimeout caps the whole reload conversation. Each read extends the
// deadline, so a slow-but-progressing reload is not mistaken for a failure.
const (
	reloadTimeout     = 60 * time.Second
	reloadReadTimeout = 15 * time.Second
)

func (r *Reloader) reload() error {
	if r.cfg.MasterSocket == "" {
		return fmt.Errorf("gateway master socket is not configured")
	}
	// Ask the running master-worker process to re-read the config from disk.
	// Do not spawn a separate haproxy binary here: the API image may ship a
	// different HAProxy version, which breaks the -x/-S handoff.
	conn, err := net.DialTimeout("unix", r.cfg.MasterSocket, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect master socket: %w", err)
	}
	defer conn.Close()

	if err := conn.SetWriteDeadline(time.Now().Add(reloadReadTimeout)); err != nil {
		return fmt.Errorf("set master socket deadline: %w", err)
	}
	if _, err := conn.Write([]byte("reload\n")); err != nil {
		return fmt.Errorf("send reload command: %w", err)
	}

	overall := time.Now().Add(reloadTimeout)
	var resp strings.Builder
	scanner := bufio.NewScanner(conn)
	for {
		deadline := time.Now().Add(reloadReadTimeout)
		if deadline.After(overall) {
			deadline = overall
		}
		if err := conn.SetReadDeadline(deadline); err != nil {
			return fmt.Errorf("set master socket deadline: %w", err)
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		resp.WriteString(line)
		resp.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read reload response: %w", err)
	}
	return parseReloadResponse(resp.String())
}

// parseReloadResponse interprets the master CLI's reply. HAProxy reports the
// outcome explicitly ("Success=1"/"Success=0"), so prefer that over scanning
// for severity tags — the reload output also echoes warnings from unrelated
// backends, which must not be mistaken for failure.
func parseReloadResponse(resp string) error {
	trimmed := strings.TrimSpace(resp)
	lower := strings.ToLower(trimmed)

	switch {
	case strings.Contains(lower, "success=1"):
		return nil
	case strings.Contains(lower, "success=0"):
		return fmt.Errorf("haproxy reload failed: %s", trimmed)
	case strings.Contains(lower, "unknown command"):
		return fmt.Errorf("haproxy master socket rejected reload: %s", trimmed)
	}

	// Older masters answer with no explicit status. Fall back to severity tags,
	// which are only emitted on the reload's own failure path.
	if strings.Contains(lower, "[alert]") || strings.Contains(lower, "[fatal]") {
		return fmt.Errorf("haproxy reload failed: %s", trimmed)
	}
	return nil
}
