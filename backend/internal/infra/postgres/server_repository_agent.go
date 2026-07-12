package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

var _ serverdom.AgentRepository = (*ServerRepository)(nil)

// SetAgentToken stores the hashed agent token and marks the server enrolled.
func (r *ServerRepository) SetAgentToken(ctx context.Context, id uuid.UUID, tokenHash string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE servers SET agent_token_hash = $2, agent_enrolled_at = now(),
			status = 'online', last_heartbeat_at = now(), version = version + 1
		WHERE id = $1 AND deleted_at IS NULL`, id, tokenHash)
	if err != nil {
		return apperr.Internal(fmt.Errorf("set agent token: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("server not found")
	}
	return nil
}

// GetByAgentTokenHash resolves the server for an agent bearer token.
func (r *ServerRepository) GetByAgentTokenHash(ctx context.Context, tokenHash string) (*serverdom.Server, error) {
	q := `SELECT ` + serverColumns + ` FROM servers WHERE agent_token_hash = $1 AND deleted_at IS NULL`
	s, err := scanServer(r.pool.QueryRow(ctx, q, tokenHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.Unauthorized("unknown agent token")
		}
		return nil, apperr.Internal(fmt.Errorf("get server by agent token: %w", err))
	}
	return s, nil
}

// Heartbeat updates liveness, versions and the health snapshot.
func (r *ServerRepository) Heartbeat(ctx context.Context, id uuid.UUID, info serverdom.HeartbeatInfo) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE servers SET status = 'online', last_heartbeat_at = now(),
			agent_version = $2,
			address = COALESCE($3::inet, address),
			mariadb_version = COALESCE($4, mariadb_version),
			os = COALESCE($5, os),
			version = version + 1
		WHERE id = $1 AND deleted_at IS NULL`,
		id, info.AgentVersion, info.Address, info.MariaDBVersion, info.OS)
	if err != nil {
		return apperr.Internal(fmt.Errorf("heartbeat servers: %w", err))
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO server_health (server_id, cpu_pct, mem_used_bytes, mem_total_bytes, disk_used_bytes, disk_total_bytes, active_connections, docker_ok, collected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (server_id) DO UPDATE SET
			cpu_pct = EXCLUDED.cpu_pct,
			mem_used_bytes = EXCLUDED.mem_used_bytes,
			mem_total_bytes = EXCLUDED.mem_total_bytes,
			disk_used_bytes = EXCLUDED.disk_used_bytes,
			disk_total_bytes = EXCLUDED.disk_total_bytes,
			active_connections = EXCLUDED.active_connections,
			docker_ok = EXCLUDED.docker_ok,
			collected_at = now()`,
		id, info.CPUPct, info.MemUsedBytes, info.MemTotalBytes, info.DiskUsedBytes, info.DiskTotalBytes, info.ActiveConnections, info.DockerOK)
	if err != nil {
		return apperr.Internal(fmt.Errorf("heartbeat health: %w", err))
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO server_health_history (server_id, cpu_pct, mem_used_bytes, mem_total_bytes, disk_used_bytes, disk_total_bytes, active_connections)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, info.CPUPct, info.MemUsedBytes, info.MemTotalBytes, info.DiskUsedBytes, info.DiskTotalBytes, info.ActiveConnections)
	if err != nil {
		return apperr.Internal(fmt.Errorf("heartbeat history: %w", err))
	}
	return nil
}

// MarkOffline flips servers whose heartbeat is older than cutoff and returns
// the ids that changed.
func (r *ServerRepository) MarkOffline(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE servers SET status = 'offline', version = version + 1
		WHERE status = 'online' AND deleted_at IS NULL
		  AND (last_heartbeat_at IS NULL OR last_heartbeat_at < $1)
		RETURNING id`, cutoff)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("mark offline: %w", err))
	}
	defer rows.Close()
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, apperr.Internal(err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

const healthSampleColumns = `cpu_pct, mem_used_bytes, mem_total_bytes, disk_used_bytes, disk_total_bytes, active_connections, collected_at`

// LatestHealthAll returns the current health snapshot for each server, with
// the server name.
func (r *ServerRepository) LatestHealthAll(ctx context.Context) ([]serverdom.HealthSample, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT h.server_id, s.name, `+healthSampleColumns+`
		FROM server_health h JOIN servers s ON s.id = h.server_id
		WHERE s.deleted_at IS NULL`)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("latest health: %w", err))
	}
	defer rows.Close()
	out := make([]serverdom.HealthSample, 0)
	for rows.Next() {
		var h serverdom.HealthSample
		if err := rows.Scan(&h.ServerID, &h.ServerName, &h.CPUPct, &h.MemUsedBytes, &h.MemTotalBytes,
			&h.DiskUsedBytes, &h.DiskTotalBytes, &h.ActiveConnections, &h.CollectedAt); err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// HealthHistory returns samples for one server since the given time (ascending).
func (r *ServerRepository) HealthHistory(ctx context.Context, id uuid.UUID, since time.Time) ([]serverdom.HealthSample, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT server_id, `+healthSampleColumns+`
		FROM server_health_history WHERE server_id = $1 AND collected_at >= $2
		ORDER BY collected_at`, id, since)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("health history: %w", err))
	}
	defer rows.Close()
	out := make([]serverdom.HealthSample, 0)
	for rows.Next() {
		var h serverdom.HealthSample
		if err := rows.Scan(&h.ServerID, &h.CPUPct, &h.MemUsedBytes, &h.MemTotalBytes,
			&h.DiskUsedBytes, &h.DiskTotalBytes, &h.ActiveConnections, &h.CollectedAt); err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// PruneHealthHistory deletes history samples collected before cutoff.
func (r *ServerRepository) PruneHealthHistory(ctx context.Context, cutoff time.Time) (int, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM server_health_history WHERE collected_at < $1`, cutoff)
	if err != nil {
		return 0, apperr.Internal(fmt.Errorf("prune health history: %w", err))
	}
	return int(tag.RowsAffected()), nil
}
