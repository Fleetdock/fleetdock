package server

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// HeartbeatInfo is the payload an agent reports on each heartbeat.
type HeartbeatInfo struct {
	AgentVersion      string
	Address           *string
	MariaDBVersion    *string
	OS                *string
	CPUPct            *float64
	MemUsedBytes      *int64
	MemTotalBytes     *int64
	DiskUsedBytes     *int64
	DiskTotalBytes    *int64
	ActiveConnections *int
	DockerOK          *bool
}

// HealthSample is one health measurement of a server at a point in time.
type HealthSample struct {
	ServerID          uuid.UUID
	ServerName        string
	CPUPct            *float64
	MemUsedBytes      *int64
	MemTotalBytes     *int64
	DiskUsedBytes     *int64
	DiskTotalBytes    *int64
	ActiveConnections *int
	CollectedAt       time.Time
}

// AgentRepository extends the persistence port with agent-enrollment and
// health operations (implemented by the same Postgres adapter).
type AgentRepository interface {
	// SetAgentToken stores the sha256 hash of the agent's bearer token and
	// marks the server enrolled.
	SetAgentToken(ctx context.Context, id uuid.UUID, tokenHash string) error
	// GetByAgentTokenHash resolves the server for an agent bearer token.
	GetByAgentTokenHash(ctx context.Context, tokenHash string) (*Server, error)
	// Heartbeat updates liveness, versions and the health snapshot, and
	// appends a history sample.
	Heartbeat(ctx context.Context, id uuid.UUID, info HeartbeatInfo) error
	// MarkOffline flips servers whose heartbeat is older than cutoff, and
	// returns the ids that were flipped.
	MarkOffline(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error)
	// LatestHealthAll returns the most recent health snapshot for each server.
	LatestHealthAll(ctx context.Context) ([]HealthSample, error)
	// HealthHistory returns samples for one server since the given time.
	HealthHistory(ctx context.Context, id uuid.UUID, since time.Time) ([]HealthSample, error)
	// PruneHealthHistory deletes history samples collected before cutoff.
	PruneHealthHistory(ctx context.Context, cutoff time.Time) (int, error)
}
