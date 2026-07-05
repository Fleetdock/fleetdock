package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	statsdom "github.com/mariadb-cp/db-manager/backend/internal/domain/stats"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
)

// StatsRepository is the Postgres adapter for statsdom.Repository.
type StatsRepository struct {
	pool *pgxpool.Pool
}

// NewStatsRepository builds a stats repository.
func NewStatsRepository(pool *pgxpool.Pool) *StatsRepository { return &StatsRepository{pool: pool} }

var _ statsdom.Repository = (*StatsRepository)(nil)

// Summary runs the aggregate queries behind the overview dashboard in one
// round-trip using a single SELECT of scalar sub-queries.
func (r *StatsRepository) Summary(ctx context.Context) (statsdom.Summary, error) {
	const q = `
		SELECT
			(SELECT count(*) FROM servers WHERE deleted_at IS NULL),
			(SELECT count(*) FROM servers WHERE deleted_at IS NULL AND status = 'online'),
			(SELECT count(*) FROM servers WHERE deleted_at IS NULL AND status = 'offline'),
			(SELECT count(*) FROM instances WHERE deleted_at IS NULL),
			(SELECT count(*) FROM instances WHERE deleted_at IS NULL AND kind = 'managed'),
			(SELECT count(*) FROM instances WHERE deleted_at IS NULL AND kind = 'external'),
			(SELECT count(*) FROM databases WHERE deleted_at IS NULL),
			(SELECT count(*) FROM databases WHERE deleted_at IS NULL AND status = 'active'),
			(SELECT count(*) FROM backups WHERE status = 'completed' AND created_at > now() - interval '24 hours'),
			(SELECT count(*) FROM backups WHERE status = 'failed' AND created_at > now() - interval '24 hours'),
			(SELECT max(completed_at) FROM backups WHERE status = 'completed'),
			(SELECT count(*) FROM jobs WHERE status = 'running'),
			(SELECT count(*) FROM jobs WHERE status = 'failed' AND created_at > now() - interval '24 hours'),
			(SELECT count(*) FROM backup_schedules WHERE enabled),
			(SELECT count(*) FROM notification_channels WHERE enabled),
			(SELECT count(*) FROM alert_rules WHERE enabled)`
	var s statsdom.Summary
	err := r.pool.QueryRow(ctx, q).Scan(
		&s.ServersTotal, &s.ServersOnline, &s.ServersOffline,
		&s.InstancesTotal, &s.InstancesManaged, &s.InstancesExternal,
		&s.DatabasesTotal, &s.DatabasesActive,
		&s.BackupsCompleted24h, &s.BackupsFailed24h, &s.LastBackupAt,
		&s.OperationsRunning, &s.OperationsFailed24h,
		&s.SchedulesEnabled, &s.ChannelsEnabled, &s.RulesEnabled,
	)
	if err != nil {
		return statsdom.Summary{}, apperr.Internal(fmt.Errorf("summary: %w", err))
	}
	return s, nil
}
