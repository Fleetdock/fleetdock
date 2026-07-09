package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authz "github.com/TajBrains/db-manager/backend/internal/domain/authz"
	userdom "github.com/TajBrains/db-manager/backend/internal/domain/user"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// UserRepository is the Postgres adapter for userdom.Repository.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository builds a user repository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository { return &UserRepository{pool: pool} }

var _ userdom.Repository = (*UserRepository)(nil)

func (r *UserRepository) GetCredentialsByEmail(ctx context.Context, email string) (userdom.Credentials, error) {
	const q = `
		SELECT id, email, name, status, created_at, updated_at, version, password_hash
		FROM users WHERE email = $1`
	var (
		c    userdom.Credentials
		hash *string
	)
	err := r.pool.QueryRow(ctx, q, email).Scan(
		&c.User.ID, &c.User.Email, &c.User.Name, &c.User.Status,
		&c.User.CreatedAt, &c.User.UpdatedAt, &c.User.Version, &hash,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return userdom.Credentials{}, apperr.NotFound("user not found")
		}
		return userdom.Credentials{}, apperr.Internal(fmt.Errorf("get credentials: %w", err))
	}
	if hash == nil {
		return userdom.Credentials{}, apperr.NotFound("user has no password set")
	}
	c.Hash = *hash
	return c, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (userdom.User, error) {
	const q = `
		SELECT id, email, name, status, created_at, updated_at, version
		FROM users WHERE id = $1`
	var u userdom.User
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&u.ID, &u.Email, &u.Name, &u.Status, &u.CreatedAt, &u.UpdatedAt, &u.Version,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return userdom.User{}, apperr.NotFound("user not found")
		}
		return userdom.User{}, apperr.Internal(fmt.Errorf("get user: %w", err))
	}
	return u, nil
}

func (r *UserRepository) GrantsFor(ctx context.Context, id uuid.UUID) ([]authz.Grant, error) {
	const q = `
		SELECT DISTINCT rp.permission, ur.scope_type, ur.scope_id
		FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		WHERE ur.user_id = $1`
	rows, err := r.pool.Query(ctx, q, id)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("grants: %w", err))
	}
	defer rows.Close()
	grants := make([]authz.Grant, 0)
	for rows.Next() {
		var (
			perm      string
			scopeType string
			scopeID   *uuid.UUID
		)
		if err := rows.Scan(&perm, &scopeType, &scopeID); err != nil {
			return nil, apperr.Internal(err)
		}
		g := authz.Grant{Permission: perm, Scope: authz.Scope{Type: authz.ScopeType(scopeType)}}
		if scopeID != nil {
			g.Scope.ID = *scopeID
		}
		grants = append(grants, g)
	}
	return grants, rows.Err()
}

func (r *UserRepository) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return 0, apperr.Internal(fmt.Errorf("count users: %w", err))
	}
	return n, nil
}

func (r *UserRepository) CreateWithRole(ctx context.Context, u *userdom.User, passwordHash, roleName string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return apperr.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort on the non-commit path

	err = tx.QueryRow(ctx,
		`INSERT INTO users (id, email, name, password_hash, status)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING created_at, updated_at, version`,
		u.ID, u.Email, u.Name, passwordHash, u.Status,
	).Scan(&u.CreatedAt, &u.UpdatedAt, &u.Version)
	if err != nil {
		return apperr.Internal(fmt.Errorf("insert user: %w", err))
	}

	var roleID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE name = $1`, roleName).Scan(&roleID); err != nil {
		return apperr.Internal(fmt.Errorf("find role %q: %w", roleName, err))
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_roles (id, user_id, role_id, scope_type) VALUES ($1, $2, $3, 'global')`,
		uuid.New(), u.ID, roleID,
	); err != nil {
		return apperr.Internal(fmt.Errorf("grant role: %w", err))
	}
	return tx.Commit(ctx)
}
