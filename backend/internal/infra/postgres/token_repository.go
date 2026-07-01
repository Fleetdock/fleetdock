package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	tokendom "github.com/mariadb-cp/db-manager/backend/internal/domain/token"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
)

// TokenRepository is the Postgres adapter for tokendom.Repository.
type TokenRepository struct {
	pool *pgxpool.Pool
}

// NewTokenRepository builds a token repository.
func NewTokenRepository(pool *pgxpool.Pool) *TokenRepository { return &TokenRepository{pool: pool} }

var _ tokendom.Repository = (*TokenRepository)(nil)

func (r *TokenRepository) Create(ctx context.Context, t *tokendom.Token, hash string) error {
	const q = `
		INSERT INTO api_tokens (id, user_id, name, prefix, token_hash, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`
	if err := r.pool.QueryRow(ctx, q,
		t.ID, t.UserID, t.Name, t.Prefix, hash, t.Scopes, t.ExpiresAt,
	).Scan(&t.CreatedAt); err != nil {
		return apperr.Internal(fmt.Errorf("insert token: %w", err))
	}
	return nil
}

func (r *TokenRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]*tokendom.Token, error) {
	const q = `
		SELECT id, user_id, name, prefix, scopes, last_used_at, expires_at, revoked_at, created_at
		FROM api_tokens
		WHERE user_id = $1
		ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list tokens: %w", err))
	}
	defer rows.Close()

	out := make([]*tokendom.Token, 0)
	for rows.Next() {
		var t tokendom.Token
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.Scopes,
			&t.LastUsedAt, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt,
		); err != nil {
			return nil, apperr.Internal(err)
		}
		if t.Scopes == nil {
			t.Scopes = []string{}
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (r *TokenRepository) Revoke(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE api_tokens SET revoked_at = now()
		 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return apperr.Internal(fmt.Errorf("revoke token: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("token not found")
	}
	return nil
}

func (r *TokenRepository) ResolveHash(ctx context.Context, hash string) (tokendom.Lookup, error) {
	const q = `
		SELECT user_id, scopes FROM api_tokens
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())`
	var l tokendom.Lookup
	if err := r.pool.QueryRow(ctx, q, hash).Scan(&l.UserID, &l.Scopes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tokendom.Lookup{}, apperr.NotFound("token not found")
		}
		return tokendom.Lookup{}, apperr.Internal(fmt.Errorf("resolve token: %w", err))
	}
	// Best-effort last-used bookkeeping; ignore failures.
	_, _ = r.pool.Exec(ctx, `UPDATE api_tokens SET last_used_at = now() WHERE token_hash = $1`, hash)
	return l, nil
}
