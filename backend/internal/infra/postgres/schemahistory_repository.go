package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	schemahistorydom "github.com/Fleetdock/fleetdock/backend/internal/domain/schemahistory"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// SchemaHistoryRepository is the Postgres adapter for schemahistorydom.Repository.
type SchemaHistoryRepository struct {
	pool *pgxpool.Pool
}

// NewSchemaHistoryRepository builds the repository.
func NewSchemaHistoryRepository(pool *pgxpool.Pool) *SchemaHistoryRepository {
	return &SchemaHistoryRepository{pool: pool}
}

var _ schemahistorydom.Repository = (*SchemaHistoryRepository)(nil)

const watchColumns = `database_id, enabled, last_checked_at, last_error, updated_at`

func scanWatch(row pgx.Row) (*schemahistorydom.Watch, error) {
	var w schemahistorydom.Watch
	return &w, row.Scan(&w.DatabaseID, &w.Enabled, &w.LastCheckedAt, &w.LastError, &w.UpdatedAt)
}

func (r *SchemaHistoryRepository) GetWatch(ctx context.Context, databaseID uuid.UUID) (*schemahistorydom.Watch, error) {
	w, err := scanWatch(r.pool.QueryRow(ctx, `SELECT `+watchColumns+` FROM schema_watches WHERE database_id = $1`, databaseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("get schema watch: %w", err))
	}
	return w, nil
}

func (r *SchemaHistoryRepository) SetWatch(ctx context.Context, databaseID uuid.UUID, enabled bool, by *uuid.UUID) (*schemahistorydom.Watch, error) {
	w, err := scanWatch(r.pool.QueryRow(ctx, `
		INSERT INTO schema_watches (database_id, enabled, updated_by) VALUES ($1, $2, $3)
		ON CONFLICT (database_id) DO UPDATE SET enabled = EXCLUDED.enabled, updated_by = EXCLUDED.updated_by, updated_at = now()
		RETURNING `+watchColumns, databaseID, enabled, by))
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("set schema watch: %w", err))
	}
	return w, nil
}

func (r *SchemaHistoryRepository) ListDue(ctx context.Context, checkedBefore time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT w.database_id FROM schema_watches w JOIN databases d ON d.id = w.database_id
		WHERE w.enabled AND (w.last_checked_at IS NULL OR w.last_checked_at < $1)
		ORDER BY w.last_checked_at NULLS FIRST LIMIT $2`, checkedBefore, limit)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list due schema watches: %w", err))
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func (r *SchemaHistoryRepository) MarkChecked(ctx context.Context, databaseID uuid.UUID, at time.Time, errMsg string) error {
	var e *string
	if errMsg != "" {
		e = &errMsg
	}
	_, err := r.pool.Exec(ctx, `UPDATE schema_watches SET last_checked_at = $2, last_error = $3 WHERE database_id = $1`,
		databaseID, at, e)
	if err != nil {
		return apperr.Internal(fmt.Errorf("mark schema watch checked: %w", err))
	}
	return nil
}

const snapshotColumns = `id, database_id, hash, summary, previous_id, trigger, taken_by, taken_at`

func scanSnapshot(row pgx.Row, withSchema bool) (*schemahistorydom.Snapshot, error) {
	var s schemahistorydom.Snapshot
	var summary []byte
	dest := []any{&s.ID, &s.DatabaseID, &s.Hash, &summary, &s.PreviousID, &s.Trigger, &s.TakenBy, &s.TakenAt}
	if withSchema {
		dest = append(dest, &s.SchemaGz)
	}
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(summary, &s.Summary); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SchemaHistoryRepository) Latest(ctx context.Context, databaseID uuid.UUID) (*schemahistorydom.Snapshot, error) {
	s, err := scanSnapshot(r.pool.QueryRow(ctx, `SELECT `+snapshotColumns+`, schema_gz FROM schema_snapshots
		WHERE database_id = $1 ORDER BY taken_at DESC LIMIT 1`, databaseID), true)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("latest schema snapshot: %w", err))
	}
	return s, nil
}

func (r *SchemaHistoryRepository) Add(ctx context.Context, s *schemahistorydom.Snapshot) error {
	summary, err := json.Marshal(s.Summary)
	if err != nil {
		return apperr.Internal(err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return apperr.Internal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := tx.QueryRow(ctx, `
		INSERT INTO schema_snapshots (database_id, hash, schema_gz, summary, previous_id, trigger, taken_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, taken_at`,
		s.DatabaseID, s.Hash, s.SchemaGz, summary, s.PreviousID, s.Trigger, s.TakenBy).Scan(&s.ID, &s.TakenAt); err != nil {
		return apperr.Internal(fmt.Errorf("add schema snapshot: %w", err))
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM schema_snapshots WHERE database_id = $1 AND id NOT IN (
			SELECT id FROM schema_snapshots WHERE database_id = $1 ORDER BY taken_at DESC LIMIT $2)`,
		s.DatabaseID, schemahistorydom.Keep); err != nil {
		return apperr.Internal(fmt.Errorf("prune schema snapshots: %w", err))
	}
	return tx.Commit(ctx)
}

func (r *SchemaHistoryRepository) List(ctx context.Context, databaseID uuid.UUID, limit int) ([]schemahistorydom.Snapshot, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+snapshotColumns+` FROM schema_snapshots
		WHERE database_id = $1 ORDER BY taken_at DESC LIMIT $2`, databaseID, limit)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list schema snapshots: %w", err))
	}
	defer rows.Close()
	out := []schemahistorydom.Snapshot{}
	for rows.Next() {
		s, err := scanSnapshot(rows, false)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (r *SchemaHistoryRepository) Get(ctx context.Context, databaseID, id uuid.UUID) (*schemahistorydom.Snapshot, error) {
	s, err := scanSnapshot(r.pool.QueryRow(ctx, `SELECT `+snapshotColumns+`, schema_gz FROM schema_snapshots
		WHERE database_id = $1 AND id = $2`, databaseID, id), true)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("schema snapshot not found")
	}
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("get schema snapshot: %w", err))
	}
	return s, nil
}
