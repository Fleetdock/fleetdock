package gateway

import (
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Proxy types reported in the "svname" column of `show stat`.
const (
	proxyFrontend = "FRONTEND"
	proxyBackend  = "BACKEND"
)

// ProxyStat is one row of HAProxy's statistics output.
type ProxyStat struct {
	// Name is the proxy (frontend/backend) name, e.g. "fe_15432".
	Name string
	// Server is the server name within a backend, or FRONTEND/BACKEND for the
	// aggregate rows.
	Server string
	// Status is HAProxy's own view: OPEN, UP, DOWN, NOLB, MAINT...
	Status string
	// SessionsTotal counts sessions that were accepted and proxied.
	SessionsTotal int64
	// DeniedConn counts connections rejected by a tcp-request rule — i.e. by
	// the endpoint's CIDR allowlist.
	DeniedConn int64
}

// IsUp reports whether HAProxy currently considers this server usable.
func (p ProxyStat) IsUp() bool {
	// HAProxy decorates transitional states, e.g. "UP 1/3" or "DOWN (agent)".
	return strings.HasPrefix(p.Status, "UP") || p.Status == "OPEN" || p.Status == "no check"
}

// Stats indexes ProxyStat rows by proxy name.
type Stats struct {
	Frontends map[string]ProxyStat
	// Servers holds the first server row of each backend. Routes are generated
	// with exactly one server ("srv1"), so this is the backend's health.
	Servers map[string]ProxyStat
}

// ShowStat queries the HAProxy admin socket for runtime statistics.
func ShowStat(socketPath string) (*Stats, error) {
	if socketPath == "" {
		return nil, fmt.Errorf("gateway admin socket is not configured")
	}
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect admin socket: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return nil, fmt.Errorf("set admin socket deadline: %w", err)
	}
	if _, err := conn.Write([]byte("show stat\n")); err != nil {
		return nil, fmt.Errorf("send show stat: %w", err)
	}
	return ParseStats(conn)
}

// ParseStats reads HAProxy's CSV statistics. Columns are located by header name
// rather than position, since the layout grows between HAProxy releases.
func ParseStats(r io.Reader) (*Stats, error) {
	rd := csv.NewReader(r)
	rd.FieldsPerRecord = -1 // trailing empty field, and rows vary by proxy type
	rows, err := rd.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse stats csv: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("empty stats response")
	}

	index, err := headerIndex(rows[0])
	if err != nil {
		return nil, err
	}

	out := &Stats{
		Frontends: make(map[string]ProxyStat),
		Servers:   make(map[string]ProxyStat),
	}
	for _, row := range rows[1:] {
		name := field(row, index, "pxname")
		svname := field(row, index, "svname")
		if name == "" || svname == "" {
			continue
		}
		stat := ProxyStat{
			Name:          name,
			Server:        svname,
			Status:        field(row, index, "status"),
			SessionsTotal: number(field(row, index, "stot")),
			DeniedConn:    number(field(row, index, "dcon")),
		}
		switch svname {
		case proxyFrontend:
			out.Frontends[name] = stat
		case proxyBackend:
			// The aggregate backend row carries no per-server check state;
			// the server row below is the useful one.
		default:
			if _, seen := out.Servers[name]; !seen {
				out.Servers[name] = stat
			}
		}
	}
	return out, nil
}

// headerIndex maps column names to positions from the leading "# ..." row.
func headerIndex(header []string) (map[string]int, error) {
	if len(header) == 0 || !strings.HasPrefix(header[0], "#") {
		return nil, fmt.Errorf("stats response is missing its header row")
	}
	index := make(map[string]int, len(header))
	for i, name := range header {
		name = strings.TrimSpace(strings.TrimPrefix(name, "#"))
		if name != "" {
			index[name] = i
		}
	}
	if _, ok := index["pxname"]; !ok {
		return nil, fmt.Errorf("stats header has no pxname column")
	}
	return index, nil
}

func field(row []string, index map[string]int, name string) string {
	i, ok := index[name]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func number(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// FrontendName is the generated frontend name for a public listen port.
func FrontendName(port int) string { return fmt.Sprintf("fe_%d", port) }

// BackendName is the generated backend name for a public listen port.
func BackendName(port int) string { return fmt.Sprintf("be_%d", port) }
