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

	rowchangedom "github.com/Fleetdock/fleetdock/backend/internal/domain/rowchange"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// RowChangeRepository is the Postgres adapter for rowchangedom.Repository.
type RowChangeRepository struct {
	pool *pgxpool.Pool
}

// NewRowChangeRepository builds the repository.
func NewRowChangeRepository(pool *pgxpool.Pool) *RowChangeRepository {
	return &RowChangeRepository{pool: pool}
}

var _ rowchangedom.Repository = (*RowChangeRepository)(nil)

// imageJSON encodes an image for a jsonb column (nil stays SQL NULL).
func imageJSON(im rowchangedom.Image) ([]byte, error) {
	if im == nil {
		return nil, nil
	}
	return json.Marshal(im)
}

func (r *RowChangeRepository) Add(ctx context.Context, c *rowchangedom.Change) error {
	key, err := imageJSON(c.Key)
	if err != nil {
		return apperr.Internal(err)
	}
	before, err := imageJSON(c.Before)
	if err != nil {
		return apperr.Internal(err)
	}
	after, err := imageJSON(c.After)
	if err != nil {
		return apperr.Internal(err)
	}
	session, err := json.Marshal(c.Session)
	if err != nil {
		return apperr.Internal(err)
	}
	var note *string
	if c.Note != "" {
		note = &c.Note
	}
	err = r.pool.QueryRow(ctx, `
		INSERT INTO row_changes (database_id, table_name, op, row_key, before, after, columns, session,
		                         undoable, note, batch_id, source, user_id, undo_of)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, created_at`,
		c.DatabaseID, c.Table, c.Op, key, before, after, c.Columns, session,
		c.Undoable, note, c.BatchID, c.Source, c.UserID, c.UndoOf).Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		return apperr.Internal(fmt.Errorf("add row change: %w", err))
	}
	return nil
}

const rowChangeColumns = `c.id, c.database_id, c.table_name, c.op, c.row_key, c.before, c.after, c.columns, c.session,
	c.undoable, coalesce(c.note, ''), c.batch_id, c.source, c.user_id, coalesce(u.email, ''), c.undo_of,
	c.undone_at, c.undone_by, c.created_at`

func scanRowChange(row pgx.Row) (*rowchangedom.Change, error) {
	var c rowchangedom.Change
	var key, before, after, session []byte
	if err := row.Scan(&c.ID, &c.DatabaseID, &c.Table, &c.Op, &key, &before, &after, &c.Columns, &session,
		&c.Undoable, &c.Note, &c.BatchID, &c.Source, &c.UserID, &c.UserEmail, &c.UndoOf,
		&c.UndoneAt, &c.UndoneBy, &c.CreatedAt); err != nil {
		return nil, err
	}
	for _, f := range []struct {
		raw []byte
		dst *rowchangedom.Image
	}{{key, &c.Key}, {before, &c.Before}, {after, &c.After}} {
		if f.raw != nil {
			if err := json.Unmarshal(f.raw, f.dst); err != nil {
				return nil, err
			}
		}
	}
	if err := json.Unmarshal(session, &c.Session); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *RowChangeRepository) Get(ctx context.Context, id uuid.UUID) (*rowchangedom.Change, error) {
	c, err := scanRowChange(r.pool.QueryRow(ctx, `SELECT `+rowChangeColumns+`
		FROM row_changes c LEFT JOIN users u ON u.id = c.user_id WHERE c.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("change not found")
	}
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("get row change: %w", err))
	}
	return c, nil
}

func (r *RowChangeRepository) List(ctx context.Context, f rowchangedom.ListFilter) ([]rowchangedom.Change, error) {
	if f.Limit <= 0 || f.Limit > rowchangedom.MaxBatchUndo {
		f.Limit = 50
	}
	q := `SELECT ` + rowChangeColumns + ` FROM row_changes c LEFT JOIN users u ON u.id = c.user_id
		WHERE c.database_id = $1`
	args := []any{f.DatabaseID}
	if f.Table != "" {
		args = append(args, f.Table)
		q += fmt.Sprintf(" AND c.table_name = $%d", len(args))
	}
	if f.BatchID != nil {
		args = append(args, *f.BatchID)
		q += fmt.Sprintf(" AND c.batch_id = $%d", len(args))
	}
	if f.Before != nil {
		args = append(args, *f.Before)
		q += fmt.Sprintf(" AND c.created_at < $%d", len(args))
	}
	args = append(args, f.Limit)
	q += fmt.Sprintf(" ORDER BY c.created_at DESC, c.id DESC LIMIT $%d", len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list row changes: %w", err))
	}
	defer rows.Close()
	out := []rowchangedom.Change{}
	for rows.Next() {
		c, err := scanRowChange(rows)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *RowChangeRepository) ClaimUndo(ctx context.Context, id, user uuid.UUID, at time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE row_changes SET undone_at = $2, undone_by = $3
		WHERE id = $1 AND undone_at IS NULL AND undoable`, id, at, user)
	if err != nil {
		return false, apperr.Internal(fmt.Errorf("claim undo: %w", err))
	}
	return tag.RowsAffected() == 1, nil
}

func (r *RowChangeRepository) ReleaseUndo(ctx context.Context, id uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, `UPDATE row_changes SET undone_at = NULL, undone_by = NULL WHERE id = $1`, id); err != nil {
		return apperr.Internal(fmt.Errorf("release undo: %w", err))
	}
	return nil
}

// PruneBefore deletes in batches so a large backlog never holds a long lock.
func (r *RowChangeRepository) PruneBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	var total int64
	for {
		tag, err := r.pool.Exec(ctx, `DELETE FROM row_changes WHERE id IN (
			SELECT id FROM row_changes WHERE created_at < $1 LIMIT 5000)`, cutoff)
		if err != nil {
			return total, apperr.Internal(fmt.Errorf("prune row changes: %w", err))
		}
		total += tag.RowsAffected()
		if tag.RowsAffected() < 5000 {
			return total, nil
		}
	}
}
