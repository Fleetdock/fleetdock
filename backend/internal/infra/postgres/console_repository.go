package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	consoledom "github.com/Fleetdock/fleetdock/backend/internal/domain/console"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// ConsoleRepository is the Postgres adapter for consoledom.Repository.
type ConsoleRepository struct {
	pool *pgxpool.Pool
}

// NewConsoleRepository builds the repository.
func NewConsoleRepository(pool *pgxpool.Pool) *ConsoleRepository {
	return &ConsoleRepository{pool: pool}
}

var _ consoledom.Repository = (*ConsoleRepository)(nil)

// AddHistory records a run and prunes the user's history for that database
// beyond HistoryLimit.
func (r *ConsoleRepository) AddHistory(ctx context.Context, e *consoledom.HistoryEntry) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO query_history (user_id, database_id, sql, statements, duration_ms, row_count, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		e.UserID, e.DatabaseID, e.SQL, e.Statements, e.DurationMS, e.RowCount, e.Error).Scan(&e.ID, &e.CreatedAt)
	if err != nil {
		return apperr.Internal(fmt.Errorf("add query history: %w", err))
	}
	_, err = r.pool.Exec(ctx, `
		DELETE FROM query_history
		WHERE user_id = $1 AND database_id = $2 AND id NOT IN (
			SELECT id FROM query_history WHERE user_id = $1 AND database_id = $2
			ORDER BY created_at DESC LIMIT $3)`, e.UserID, e.DatabaseID, consoledom.HistoryLimit)
	if err != nil {
		return apperr.Internal(fmt.Errorf("prune query history: %w", err))
	}
	return nil
}

func (r *ConsoleRepository) ListHistory(ctx context.Context, userID, databaseID uuid.UUID, limit int) ([]consoledom.HistoryEntry, error) {
	if limit <= 0 || limit > consoledom.HistoryLimit {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, database_id, sql, statements, duration_ms, row_count, error, created_at
		FROM query_history WHERE user_id = $1 AND database_id = $2
		ORDER BY created_at DESC LIMIT $3`, userID, databaseID, limit)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list query history: %w", err))
	}
	defer rows.Close()
	out := []consoledom.HistoryEntry{}
	for rows.Next() {
		var e consoledom.HistoryEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.DatabaseID, &e.SQL, &e.Statements, &e.DurationMS, &e.RowCount, &e.Error, &e.CreatedAt); err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

const savedColumns = `id, user_id, database_id, name, sql, created_at, updated_at`

func scanSaved(row pgx.Row) (*consoledom.SavedQuery, error) {
	var q consoledom.SavedQuery
	if err := row.Scan(&q.ID, &q.UserID, &q.DatabaseID, &q.Name, &q.SQL, &q.CreatedAt, &q.UpdatedAt); err != nil {
		return nil, err
	}
	return &q, nil
}

func (r *ConsoleRepository) ListSaved(ctx context.Context, userID uuid.UUID, databaseID *uuid.UUID) ([]consoledom.SavedQuery, error) {
	// With a database: its own queries plus the user's unscoped ones.
	rows, err := r.pool.Query(ctx, `
		SELECT `+savedColumns+` FROM saved_queries
		WHERE user_id = $1 AND ($2::uuid IS NULL OR database_id IS NULL OR database_id = $2)
		ORDER BY lower(name)`, userID, databaseID)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list saved queries: %w", err))
	}
	defer rows.Close()
	out := []consoledom.SavedQuery{}
	for rows.Next() {
		q, err := scanSaved(rows)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, *q)
	}
	return out, rows.Err()
}

func (r *ConsoleRepository) CreateSaved(ctx context.Context, q *consoledom.SavedQuery) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO saved_queries (user_id, database_id, name, sql) VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`, q.UserID, q.DatabaseID, q.Name, q.SQL).Scan(&q.ID, &q.CreatedAt, &q.UpdatedAt)
	if err != nil {
		return apperr.Internal(fmt.Errorf("create saved query: %w", err))
	}
	return nil
}

func (r *ConsoleRepository) UpdateSaved(ctx context.Context, userID, id uuid.UUID, name, sql string) (*consoledom.SavedQuery, error) {
	q, err := scanSaved(r.pool.QueryRow(ctx, `
		UPDATE saved_queries SET name = $3, sql = $4, updated_at = now()
		WHERE id = $2 AND user_id = $1 RETURNING `+savedColumns, userID, id, name, sql))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("saved query not found")
		}
		return nil, apperr.Internal(fmt.Errorf("update saved query: %w", err))
	}
	return q, nil
}

func (r *ConsoleRepository) DeleteSaved(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM saved_queries WHERE id = $2 AND user_id = $1`, userID, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("delete saved query: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("saved query not found")
	}
	return nil
}
