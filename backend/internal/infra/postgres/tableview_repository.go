package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	tableviewdom "github.com/Fleetdock/fleetdock/backend/internal/domain/tableview"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// TableViewRepository is the Postgres adapter for tableviewdom.Repository.
type TableViewRepository struct {
	pool *pgxpool.Pool
}

// NewTableViewRepository builds the repository.
func NewTableViewRepository(pool *pgxpool.Pool) *TableViewRepository {
	return &TableViewRepository{pool: pool}
}

var _ tableviewdom.Repository = (*TableViewRepository)(nil)

const tableViewColumns = `v.id, v.database_id, v.table_name, v.name, v.state, v.shared, v.created_by,
	COALESCE(u.email, ''), v.created_at, v.updated_at`

func scanTableView(row pgx.Row) (*tableviewdom.View, error) {
	var v tableviewdom.View
	err := row.Scan(&v.ID, &v.DatabaseID, &v.Table, &v.Name, &v.State, &v.Shared, &v.CreatedBy,
		&v.AuthorEmail, &v.CreatedAt, &v.UpdatedAt)
	return &v, err
}

func viewConflict(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return apperr.Conflict("you already have a view with this name on this table")
	}
	return apperr.Internal(err)
}

func (r *TableViewRepository) List(ctx context.Context, databaseID uuid.UUID, table string, userID uuid.UUID) ([]tableviewdom.View, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+tableViewColumns+`
		FROM table_views v LEFT JOIN users u ON u.id = v.created_by
		WHERE v.database_id = $1 AND ($2 = '' OR v.table_name = $2) AND (v.created_by = $3 OR v.shared)
		ORDER BY v.table_name, lower(v.name), v.created_at`, databaseID, table, userID)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list table views: %w", err))
	}
	defer rows.Close()
	out := []tableviewdom.View{}
	for rows.Next() {
		v, err := scanTableView(rows)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *TableViewRepository) Get(ctx context.Context, databaseID, id uuid.UUID) (*tableviewdom.View, error) {
	v, err := scanTableView(r.pool.QueryRow(ctx, `SELECT `+tableViewColumns+`
		FROM table_views v LEFT JOIN users u ON u.id = v.created_by
		WHERE v.database_id = $1 AND v.id = $2`, databaseID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("view not found")
	}
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("get table view: %w", err))
	}
	return v, nil
}

func (r *TableViewRepository) Create(ctx context.Context, v *tableviewdom.View) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO table_views (database_id, table_name, name, state, shared, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`,
		v.DatabaseID, v.Table, v.Name, v.State, v.Shared, v.CreatedBy).Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return viewConflict(err)
	}
	return nil
}

func (r *TableViewRepository) Update(ctx context.Context, v *tableviewdom.View) error {
	err := r.pool.QueryRow(ctx, `
		UPDATE table_views SET name = $3, state = $4, shared = $5, updated_at = now()
		WHERE database_id = $1 AND id = $2
		RETURNING updated_at`, v.DatabaseID, v.ID, v.Name, v.State, v.Shared).Scan(&v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NotFound("view not found")
	}
	if err != nil {
		return viewConflict(err)
	}
	return nil
}

func (r *TableViewRepository) Delete(ctx context.Context, databaseID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM table_views WHERE database_id = $1 AND id = $2`, databaseID, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("delete table view: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("view not found")
	}
	return nil
}
