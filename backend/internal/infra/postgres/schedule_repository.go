package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	scheduledom "github.com/TajBrains/db-manager/backend/internal/domain/schedule"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// ScheduleRepository is the Postgres adapter for scheduledom.Repository.
type ScheduleRepository struct {
	pool *pgxpool.Pool
}

// NewScheduleRepository builds a schedule repository.
func NewScheduleRepository(pool *pgxpool.Pool) *ScheduleRepository {
	return &ScheduleRepository{pool: pool}
}

var _ scheduledom.Repository = (*ScheduleRepository)(nil)

const scheduleColumns = `
	id, database_id, destination_id, cron, engine, retention_days, enabled,
	last_run_at, next_run_at, created_by, created_at, updated_at, version`

func (r *ScheduleRepository) Create(ctx context.Context, s *scheduledom.Schedule) error {
	const q = `
		INSERT INTO backup_schedules
			(id, database_id, destination_id, cron, engine, retention_days, enabled, next_run_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at, version`
	err := r.pool.QueryRow(ctx, q,
		s.ID, s.DatabaseID, s.DestinationID, s.Cron, s.Engine, s.RetentionDays, s.Enabled, s.NextRunAt, s.CreatedBy,
	).Scan(&s.CreatedAt, &s.UpdatedAt, &s.Version)
	if err != nil {
		return apperr.Internal(fmt.Errorf("insert schedule: %w", err))
	}
	return nil
}

func (r *ScheduleRepository) GetByID(ctx context.Context, id uuid.UUID) (*scheduledom.Schedule, error) {
	q := `SELECT ` + scheduleColumns + ` FROM backup_schedules WHERE id = $1`
	s, err := scanSchedule(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("schedule not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get schedule: %w", err))
	}
	return s, nil
}

func (r *ScheduleRepository) List(ctx context.Context, databaseID *uuid.UUID) ([]*scheduledom.Schedule, error) {
	q := `SELECT ` + scheduleColumns + ` FROM backup_schedules`
	args := []any{}
	if databaseID != nil {
		q += ` WHERE database_id = $1`
		args = append(args, *databaseID)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list schedules: %w", err))
	}
	defer rows.Close()
	out := make([]*scheduledom.Schedule, 0)
	for rows.Next() {
		s, err := scanSchedule(rows)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *ScheduleRepository) Update(ctx context.Context, s *scheduledom.Schedule) error {
	const q = `
		UPDATE backup_schedules SET
			destination_id = $2, cron = $3, retention_days = $4, enabled = $5,
			next_run_at = $6, version = version + 1
		WHERE id = $1
		RETURNING updated_at, version`
	err := r.pool.QueryRow(ctx, q,
		s.ID, s.DestinationID, s.Cron, s.RetentionDays, s.Enabled, s.NextRunAt,
	).Scan(&s.UpdatedAt, &s.Version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("schedule not found")
		}
		return apperr.Internal(fmt.Errorf("update schedule: %w", err))
	}
	return nil
}

func (r *ScheduleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM backup_schedules WHERE id = $1`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("delete schedule: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("schedule not found")
	}
	return nil
}

func (r *ScheduleRepository) Due(ctx context.Context, now time.Time) ([]*scheduledom.Schedule, error) {
	q := `SELECT ` + scheduleColumns + `
		FROM backup_schedules
		WHERE enabled AND next_run_at IS NOT NULL AND next_run_at <= $1
		ORDER BY next_run_at`
	rows, err := r.pool.Query(ctx, q, now)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list due schedules: %w", err))
	}
	defer rows.Close()
	out := make([]*scheduledom.Schedule, 0)
	for rows.Next() {
		s, err := scanSchedule(rows)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *ScheduleRepository) MarkRun(ctx context.Context, id uuid.UUID, lastRun, nextRun time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE backup_schedules SET last_run_at = $2, next_run_at = $3, version = version + 1 WHERE id = $1`,
		id, lastRun, nextRun)
	if err != nil {
		return apperr.Internal(fmt.Errorf("mark schedule run: %w", err))
	}
	return nil
}

func scanSchedule(row rowScanner) (*scheduledom.Schedule, error) {
	var s scheduledom.Schedule
	if err := row.Scan(
		&s.ID, &s.DatabaseID, &s.DestinationID, &s.Cron, &s.Engine, &s.RetentionDays, &s.Enabled,
		&s.LastRunAt, &s.NextRunAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt, &s.Version,
	); err != nil {
		return nil, err
	}
	return &s, nil
}
