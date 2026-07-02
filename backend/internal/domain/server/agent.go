package server

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// HeartbeatInfo is the payload an agent reports on each heartbeat.
type HeartbeatInfo struct {
	AgentVersion   string
	MariaDBVersion *string
	OS             *string
	CPUPct         *float64
	MemUsedBytes   *int64
	MemTotalBytes  *int64
	DiskUsedBytes  *int64
	DiskTotalBytes *int64
	DockerOK       *bool
}

// AgentRepository extends the persistence port with agent-enrollment and
// health operations (implemented by the same Postgres adapter).
type AgentRepository interface {
	// SetAgentToken stores the sha256 hash of the agent's bearer token and
	// marks the server enrolled.
	SetAgentToken(ctx context.Context, id uuid.UUID, tokenHash string) error
	// GetByAgentTokenHash resolves the server for an agent bearer token.
	GetByAgentTokenHash(ctx context.Context, tokenHash string) (*Server, error)
	// Heartbeat updates liveness, versions and the health snapshot.
	Heartbeat(ctx context.Context, id uuid.UUID, info HeartbeatInfo) error
	// MarkOffline flips servers whose heartbeat is older than cutoff.
	MarkOffline(ctx context.Context, cutoff time.Time) (int, error)
}
