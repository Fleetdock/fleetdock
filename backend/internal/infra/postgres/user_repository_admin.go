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

var _ userdom.AdminRepository = (*UserRepository)(nil)

// List returns all users with their global role names.
func (r *UserRepository) List(ctx context.Context) ([]userdom.WithRoles, error) {
	const q = `
		SELECT u.id, u.email, u.name, u.status, u.created_at, u.updated_at, u.version,
		       COALESCE(array_agg(ro.name ORDER BY ro.name) FILTER (WHERE ro.name IS NOT NULL), '{}')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id AND ur.scope_type = 'global'
		LEFT JOIN roles ro ON ro.id = ur.role_id
		GROUP BY u.id
		ORDER BY u.created_at`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list users: %w", err))
	}
	defer rows.Close()
	out := make([]userdom.WithRoles, 0)
	for rows.Next() {
		var u userdom.WithRoles
		if err := rows.Scan(
			&u.ID, &u.Email, &u.Name, &u.Status, &u.CreatedAt, &u.UpdatedAt, &u.Version, &u.Roles,
		); err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// RolesFor returns the global role names assigned to a user.
func (r *UserRepository) RolesFor(ctx context.Context, id uuid.UUID) ([]string, error) {
	const q = `
		SELECT ro.name
		FROM user_roles ur
		JOIN roles ro ON ro.id = ur.role_id
		WHERE ur.user_id = $1 AND ur.scope_type = 'global'
		ORDER BY ro.name`
	rows, err := r.pool.Query(ctx, q, id)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("roles for user: %w", err))
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UpdateProfile changes name and/or email (nil = keep).
func (r *UserRepository) UpdateProfile(ctx context.Context, id uuid.UUID, name, email *string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE users SET
			name = COALESCE($2, name),
			email = COALESCE($3, email),
			version = version + 1
		WHERE id = $1`, id, name, email)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a user with this email already exists")
		}
		return apperr.Internal(fmt.Errorf("update profile: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("user not found")
	}
	return nil
}

// SetPassword replaces the stored password hash and advances the token epoch,
// which invalidates any JWT sessions issued before the change.
func (r *UserRepository) SetPassword(ctx context.Context, id uuid.UUID, hash string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, token_epoch = token_epoch + 1, version = version + 1 WHERE id = $1`, id, hash)
	if err != nil {
		return apperr.Internal(fmt.Errorf("set password: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("user not found")
	}
	return nil
}

// SetStatus transitions the account status.
func (r *UserRepository) SetStatus(ctx context.Context, id uuid.UUID, status string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET status = $2, version = version + 1 WHERE id = $1`, id, status)
	if err != nil {
		return apperr.Internal(fmt.Errorf("set status: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("user not found")
	}
	return nil
}

// SetGlobalRole replaces all global role grants with the named role.
func (r *UserRepository) SetGlobalRole(ctx context.Context, id uuid.UUID, roleName string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return apperr.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort on the non-commit path

	var roleID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE name = $1`, roleName).Scan(&roleID); err != nil {
		return apperr.Invalid("role", "unknown role")
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM user_roles WHERE user_id = $1 AND scope_type = 'global'`, id); err != nil {
		return apperr.Internal(fmt.Errorf("clear roles: %w", err))
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_roles (id, user_id, role_id, scope_type) VALUES ($1, $2, $3, 'global')`,
		uuid.New(), id, roleID); err != nil {
		return apperr.Internal(fmt.Errorf("grant role: %w", err))
	}
	return tx.Commit(ctx)
}

// Delete permanently removes the account.
func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("delete user: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("user not found")
	}
	return nil
}

// CountActiveOwners counts active users holding the owner role globally.
func (r *UserRepository) CountActiveOwners(ctx context.Context) (int, error) {
	const q = `
		SELECT count(DISTINCT u.id)
		FROM users u
		JOIN user_roles ur ON ur.user_id = u.id AND ur.scope_type = 'global'
		JOIN roles ro ON ro.id = ur.role_id
		WHERE ro.name = 'owner' AND u.status = 'active'`
	var n int
	if err := r.pool.QueryRow(ctx, q).Scan(&n); err != nil {
		return 0, apperr.Internal(fmt.Errorf("count owners: %w", err))
	}
	return n, nil
}

// ListGrants returns all role grants (global and scoped) for a user.
func (r *UserRepository) ListGrants(ctx context.Context, userID uuid.UUID) ([]userdom.RoleGrant, error) {
	const q = `
		SELECT ur.id, ur.role_id, ro.name, ur.scope_type, ur.scope_id
		FROM user_roles ur
		JOIN roles ro ON ro.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY ur.scope_type, ro.name`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list grants: %w", err))
	}
	defer rows.Close()
	out := make([]userdom.RoleGrant, 0)
	for rows.Next() {
		var g userdom.RoleGrant
		if err := rows.Scan(&g.ID, &g.RoleID, &g.RoleName, &g.ScopeType, &g.ScopeID); err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetGrant returns one grant belonging to a user.
func (r *UserRepository) GetGrant(ctx context.Context, userID, grantID uuid.UUID) (userdom.RoleGrant, error) {
	const q = `
		SELECT ur.id, ur.role_id, ro.name, ur.scope_type, ur.scope_id
		FROM user_roles ur
		JOIN roles ro ON ro.id = ur.role_id
		WHERE ur.id = $1 AND ur.user_id = $2`
	var g userdom.RoleGrant
	err := r.pool.QueryRow(ctx, q, grantID, userID).Scan(&g.ID, &g.RoleID, &g.RoleName, &g.ScopeType, &g.ScopeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return userdom.RoleGrant{}, apperr.NotFound("grant not found")
		}
		return userdom.RoleGrant{}, apperr.Internal(fmt.Errorf("get grant: %w", err))
	}
	return g, nil
}

// AddGrant assigns a role to a user at a scope (scopeID nil for global).
func (r *UserRepository) AddGrant(ctx context.Context, userID uuid.UUID, roleName, scopeType string, scopeID *uuid.UUID) (userdom.RoleGrant, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return userdom.RoleGrant{}, apperr.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort on the non-commit path

	var roleID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE name = $1`, roleName).Scan(&roleID); err != nil {
		return userdom.RoleGrant{}, apperr.Invalid("role", "unknown role")
	}
	id := uuid.New()
	_, err = tx.Exec(ctx,
		`INSERT INTO user_roles (id, user_id, role_id, scope_type, scope_id) VALUES ($1, $2, $3, $4, $5)`,
		id, userID, roleID, scopeType, scopeID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return userdom.RoleGrant{}, apperr.Conflict("this role is already granted at that scope")
		}
		return userdom.RoleGrant{}, apperr.Internal(fmt.Errorf("add grant: %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return userdom.RoleGrant{}, apperr.Internal(err)
	}
	return userdom.RoleGrant{ID: id, RoleID: roleID, RoleName: roleName, ScopeType: scopeType, ScopeID: scopeID}, nil
}

// RemoveGrant deletes a user's grant by id.
func (r *UserRepository) RemoveGrant(ctx context.Context, userID, grantID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM user_roles WHERE id = $1 AND user_id = $2`, grantID, userID)
	if err != nil {
		return apperr.Internal(fmt.Errorf("remove grant: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("grant not found")
	}
	return nil
}

// ListRoles returns the role catalog with permissions.
func (r *UserRepository) ListRoles(ctx context.Context) ([]userdom.Role, error) {
	const q = `
		SELECT ro.id, ro.name, ro.description, ro.is_system,
		       COALESCE(array_agg(rp.permission ORDER BY rp.permission) FILTER (WHERE rp.permission IS NOT NULL), '{}')
		FROM roles ro
		LEFT JOIN role_permissions rp ON rp.role_id = ro.id
		GROUP BY ro.id
		ORDER BY ro.name`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list roles: %w", err))
	}
	defer rows.Close()
	out := make([]userdom.Role, 0)
	for rows.Next() {
		var ro userdom.Role
		if err := rows.Scan(&ro.ID, &ro.Name, &ro.Description, &ro.IsSystem, &ro.Permissions); err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, ro)
	}
	return out, rows.Err()
}
