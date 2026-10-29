package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	catalogdom "github.com/Fleetdock/fleetdock/backend/internal/domain/catalog"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// CatalogRepository is the Postgres adapter for catalogdom.Repository.
type CatalogRepository struct {
	pool *pgxpool.Pool
}

// NewCatalogRepository builds the repository.
func NewCatalogRepository(pool *pgxpool.Pool) *CatalogRepository {
	return &CatalogRepository{pool: pool}
}

var _ catalogdom.Repository = (*CatalogRepository)(nil)

// Replace swaps the database's rows in one transaction, so a search never
// sees a half-written catalog.
func (r *CatalogRepository) Replace(ctx context.Context, databaseID uuid.UUID, cat *engine.ColumnCatalog, at time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return apperr.Internal(fmt.Errorf("begin catalog replace: %w", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM schema_catalog WHERE database_id = $1`, databaseID); err != nil {
		return apperr.Internal(fmt.Errorf("clear catalog: %w", err))
	}
	var rows [][]any
	for _, t := range cat.Tables {
		rows = append(rows, []any{databaseID, t.Schema, t.Name, t.Kind, nil, ""})
		for _, c := range t.Columns {
			rows = append(rows, []any{databaseID, t.Schema, t.Name, t.Kind, c.Name, c.Type})
		}
	}
	if len(rows) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"schema_catalog"},
			[]string{"database_id", "schema_name", "table_name", "kind", "column_name", "data_type"},
			pgx.CopyFromRows(rows)); err != nil {
			return apperr.Internal(fmt.Errorf("write catalog: %w", err))
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO schema_catalog_state (database_id, refreshed_at, attempted_at, truncated, error)
		VALUES ($1, $2, $2, $3, NULL)
		ON CONFLICT (database_id) DO UPDATE
		SET refreshed_at = $2, attempted_at = $2, truncated = $3, error = NULL`,
		databaseID, at, cat.Truncated); err != nil {
		return apperr.Internal(fmt.Errorf("record catalog refresh: %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return apperr.Internal(fmt.Errorf("commit catalog replace: %w", err))
	}
	return nil
}

// Failed keeps the previous catalog (stale names beat none) and records why
// the refresh failed.
func (r *CatalogRepository) Failed(ctx context.Context, databaseID uuid.UUID, msg string, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO schema_catalog_state (database_id, attempted_at, error) VALUES ($1, $2, $3)
		ON CONFLICT (database_id) DO UPDATE SET attempted_at = $2, error = $3`, databaseID, at, msg)
	if err != nil {
		return apperr.Internal(fmt.Errorf("record catalog failure: %w", err))
	}
	return nil
}

func (r *CatalogRepository) ListDue(ctx context.Context, staleBefore, retryBefore time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT d.id
		FROM databases d
		JOIN instances i ON i.id = d.instance_id
		LEFT JOIN schema_catalog_state s ON s.database_id = d.id
		WHERE d.deleted_at IS NULL AND d.status = 'active' AND NOT d.system
		  AND i.deleted_at IS NULL AND i.status NOT IN ('provisioning', 'deleting')
		  AND i.root_secret_ref IS NOT NULL AND coalesce(i.username, '') <> ''
		  AND (s.database_id IS NULL
		       OR ((s.refreshed_at IS NULL OR s.refreshed_at < $1) AND s.attempted_at < $2))
		ORDER BY s.attempted_at NULLS FIRST, d.id
		LIMIT $3`, staleBefore, retryBefore, limit)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list catalog due: %w", err))
	}
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// likeEscaper makes user text literal inside a LIKE pattern.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (r *CatalogRepository) Search(ctx context.Context, q string, limit int) ([]catalogdom.Hit, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return []catalogdom.Hit{}, nil
	}
	if limit <= 0 || limit > 4*catalogdom.MaxSearchLimit {
		limit = catalogdom.MaxSearchLimit
	}
	pattern := "%" + likeEscaper.Replace(q) + "%"
	// Exact names first, then prefixes, then substrings; tables before
	// columns at each level.
	rows, err := r.pool.Query(ctx, `
		SELECT c.database_id, d.name, i.id, i.name, i.engine, c.schema_name, c.table_name, c.kind,
		       c.column_name, c.data_type
		FROM schema_catalog c
		JOIN databases d ON d.id = c.database_id AND d.deleted_at IS NULL
		JOIN instances i ON i.id = d.instance_id AND i.deleted_at IS NULL
		WHERE (c.column_name IS NULL AND lower(c.table_name) LIKE $1 ESCAPE '\')
		   OR (c.column_name IS NOT NULL AND lower(c.column_name) LIKE $1 ESCAPE '\')
		ORDER BY CASE WHEN lower(coalesce(c.column_name, c.table_name)) = $2 THEN 0
		              WHEN starts_with(lower(coalesce(c.column_name, c.table_name)), $2) THEN 1 ELSE 2 END,
		         c.column_name IS NOT NULL, d.name, c.schema_name, c.table_name, c.column_name
		LIMIT $3`, pattern, q, limit)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("search catalog: %w", err))
	}
	defer rows.Close()
	out := []catalogdom.Hit{}
	for rows.Next() {
		var h catalogdom.Hit
		if err := rows.Scan(&h.DatabaseID, &h.DatabaseName, &h.InstanceID, &h.InstanceName, &h.Engine,
			&h.Schema, &h.Table, &h.Kind, &h.Column, &h.Type); err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
