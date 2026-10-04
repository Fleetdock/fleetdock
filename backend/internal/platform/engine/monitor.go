package engine

import (
	"context"
	"fmt"
)

// Process is one session/connection on an instance.
type Process struct {
	ID       int64   `json:"id"`
	User     string  `json:"user"`
	Host     string  `json:"host"`
	Database *string `json:"database"`
	// State is the command (MySQL) or state (PostgreSQL): Query, Sleep,
	// active, idle, idle in transaction, ...
	State string `json:"state"`
	// Seconds is how long the current statement (or state) has lasted.
	Seconds int64   `json:"seconds"`
	Query   *string `json:"query"`
}

// Setting is a name/value pair from server status or configuration.
type Setting struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// DatabaseStat is the size and connection count of one database.
type DatabaseStat struct {
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	Connections int    `json:"connections"`
}

// Monitor is the observability surface of an engine: sessions, status
// counters, configuration and per-database usage. Everything runs as the
// instance admin, since only it can see other sessions.
type Monitor interface {
	Processes(ctx context.Context, p ConnParams) ([]Process, error)
	// KillProcess cancels the session's current statement, or with
	// connection=true terminates the whole session.
	KillProcess(ctx context.Context, p ConnParams, id int64, connection bool) error
	ServerStatus(ctx context.Context, p ConnParams) ([]Setting, error)
	Variables(ctx context.Context, p ConnParams) ([]Setting, error)
	DatabaseStats(ctx context.Context, p ConnParams) ([]DatabaseStat, error)
}

// MonitorFor returns the monitoring surface for the engine name.
func MonitorFor(name string) (Monitor, error) {
	c, err := For(name)
	if err != nil {
		return nil, err
	}
	m, ok := c.(Monitor)
	if !ok {
		return nil, fmt.Errorf("engine: %s does not support monitoring", name)
	}
	return m, nil
}

// maxProcessQuery truncates statement text in process lists.
const maxProcessQuery = 2048
