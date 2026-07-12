package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	userdom "github.com/Fleetdock/fleetdock/backend/internal/domain/user"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// GetRole returns one role with its permissions.
func (r *UserRepository) GetRole(ctx context.Context, id uuid.UUID) (*userdom.Role, error) {
	const q = `
		SELECT ro.id, ro.name, ro.description, ro.is_system,
		       COALESCE(array_agg(rp.permission ORDER BY rp.permission) FILTER (WHERE rp.permission IS NOT NULL), '{}')
		FROM roles ro
		LEFT JOIN role_permissions rp ON rp.role_id = ro.id
		WHERE ro.id = $1
		GROUP BY ro.id`
	var ro userdom.Role
	err := r.pool.QueryRow(ctx, q, id).Scan(&ro.ID, &ro.Name, &ro.Description, &ro.IsSystem, &ro.Permissions)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("role not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get role: %w", err))
	}
	return &ro, nil
}

// CreateRole inserts a custom (non-system) role with permissions.
func (r *UserRepository) CreateRole(ctx context.Context, ro *userdom.Role) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return apperr.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort on the non-commit path

	err = tx.QueryRow(ctx,
		`INSERT INTO roles (id, name, description, is_system) VALUES ($1, $2, $3, false)
		 RETURNING id`, ro.ID, ro.Name, ro.Description).Scan(&ro.ID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a role with this name already exists")
		}
		return apperr.Internal(fmt.Errorf("insert role: %w", err))
	}
	for _, p := range ro.Permissions {
		if _, err := tx.Exec(ctx,
			`INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			ro.ID, p); err != nil {
			return apperr.Internal(fmt.Errorf("insert role permission: %w", err))
		}
	}
	return tx.Commit(ctx)
}

// UpdateRole replaces name/description/permissions of a role.
func (r *UserRepository) UpdateRole(ctx context.Context, id uuid.UUID, name, description *string, permissions []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return apperr.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort on the non-commit path

	tag, err := tx.Exec(ctx, `
		UPDATE roles SET
			name = COALESCE($2, name),
			description = COALESCE($3, description)
		WHERE id = $1`, id, name, description)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a role with this name already exists")
		}
		return apperr.Internal(fmt.Errorf("update role: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("role not found")
	}

	if permissions != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, id); err != nil {
			return apperr.Internal(fmt.Errorf("clear role permissions: %w", err))
		}
		for _, p := range permissions {
			if _, err := tx.Exec(ctx,
				`INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				id, p); err != nil {
				return apperr.Internal(fmt.Errorf("insert role permission: %w", err))
			}
		}
	}
	return tx.Commit(ctx)
}

// DeleteRole removes a role.
func (r *UserRepository) DeleteRole(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("delete role: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("role not found")
	}
	return nil
}

// CountRoleAssignments counts user_roles rows referencing the role.
func (r *UserRepository) CountRoleAssignments(ctx context.Context, id uuid.UUID) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM user_roles WHERE role_id = $1`, id).Scan(&n); err != nil {
		return 0, apperr.Internal(fmt.Errorf("count role assignments: %w", err))
	}
	return n, nil
}
