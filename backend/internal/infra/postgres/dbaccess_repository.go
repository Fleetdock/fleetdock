package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbaccessdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbaccess"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// DBAccessRepository is the Postgres adapter for dbaccessdom.Repository.
type DBAccessRepository struct {
	pool *pgxpool.Pool
}

// NewDBAccessRepository builds the repository.
func NewDBAccessRepository(pool *pgxpool.Pool) *DBAccessRepository {
	return &DBAccessRepository{pool: pool}
}

var _ dbaccessdom.Repository = (*DBAccessRepository)(nil)

const dbAccessColumns = `database_id, mode, username, secret_ref, created_at, applied_at`

func scanDBAccessRole(row pgx.Row) (*dbaccessdom.Role, error) {
	var r dbaccessdom.Role
	var mode string
	if err := row.Scan(&r.DatabaseID, &mode, &r.Username, &r.SecretRef, &r.CreatedAt, &r.AppliedAt); err != nil {
		return nil, err
	}
	r.Mode = dbaccessdom.Mode(mode)
	return &r, nil
}

func (r *DBAccessRepository) Get(ctx context.Context, databaseID uuid.UUID, mode dbaccessdom.Mode) (*dbaccessdom.Role, error) {
	role, err := scanDBAccessRole(r.pool.QueryRow(ctx,
		`SELECT `+dbAccessColumns+` FROM database_access_roles WHERE database_id = $1 AND mode = $2`,
		databaseID, string(mode)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("access role not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get access role: %w", err))
	}
	return role, nil
}

func (r *DBAccessRepository) Upsert(ctx context.Context, role *dbaccessdom.Role) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO database_access_roles (database_id, mode, username, secret_ref)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (database_id, mode) DO UPDATE
		SET username = EXCLUDED.username, secret_ref = EXCLUDED.secret_ref, applied_at = now()`,
		role.DatabaseID, string(role.Mode), role.Username, role.SecretRef)
	if err != nil {
		return apperr.Internal(fmt.Errorf("upsert access role: %w", err))
	}
	return nil
}

func (r *DBAccessRepository) Touch(ctx context.Context, databaseID uuid.UUID, mode dbaccessdom.Mode) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE database_access_roles SET applied_at = now() WHERE database_id = $1 AND mode = $2`,
		databaseID, string(mode))
	if err != nil {
		return apperr.Internal(fmt.Errorf("touch access role: %w", err))
	}
	return nil
}

func (r *DBAccessRepository) ListByDatabase(ctx context.Context, databaseID uuid.UUID) ([]*dbaccessdom.Role, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+dbAccessColumns+` FROM database_access_roles WHERE database_id = $1 ORDER BY mode`, databaseID)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list access roles: %w", err))
	}
	defer rows.Close()
	var out []*dbaccessdom.Role
	for rows.Next() {
		role, err := scanDBAccessRole(rows)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

func (r *DBAccessRepository) DeleteByDatabase(ctx context.Context, databaseID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM database_access_roles WHERE database_id = $1`, databaseID); err != nil {
		return apperr.Internal(fmt.Errorf("delete access roles: %w", err))
	}
	return nil
}
