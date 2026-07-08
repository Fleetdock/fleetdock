package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	regtokendom "github.com/TajBrains/db-manager/backend/internal/domain/regtoken"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// RegTokenRepository is the Postgres adapter for regtokendom.Repository.
type RegTokenRepository struct {
	pool *pgxpool.Pool
}

// NewRegTokenRepository builds a registration-token repository.
func NewRegTokenRepository(pool *pgxpool.Pool) *RegTokenRepository {
	return &RegTokenRepository{pool: pool}
}

var _ regtokendom.Repository = (*RegTokenRepository)(nil)

const regTokenColumns = `id, name, token_hash, created_by, expires_at, used_at, server_id, created_at`

func (r *RegTokenRepository) Create(ctx context.Context, t *regtokendom.Token) error {
	const q = `
		INSERT INTO agent_registration_tokens (id, name, token_hash, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at`
	err := r.pool.QueryRow(ctx, q, t.ID, t.Name, t.TokenHash, t.CreatedBy, t.ExpiresAt).Scan(&t.CreatedAt)
	if err != nil {
		return apperr.Internal(fmt.Errorf("insert registration token: %w", err))
	}
	return nil
}

func (r *RegTokenRepository) GetByHash(ctx context.Context, hash string) (*regtokendom.Token, error) {
	q := `SELECT ` + regTokenColumns + ` FROM agent_registration_tokens WHERE token_hash = $1`
	var t regtokendom.Token
	err := r.pool.QueryRow(ctx, q, hash).Scan(
		&t.ID, &t.Name, &t.TokenHash, &t.CreatedBy, &t.ExpiresAt, &t.UsedAt, &t.ServerID, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("registration token not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get registration token: %w", err))
	}
	return &t, nil
}

func (r *RegTokenRepository) MarkUsed(ctx context.Context, id, serverID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE agent_registration_tokens SET used_at = now(), server_id = $2
		WHERE id = $1 AND used_at IS NULL`, id, serverID)
	if err != nil {
		return apperr.Internal(fmt.Errorf("mark registration token used: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.Conflict("registration token already used")
	}
	return nil
}

func (r *RegTokenRepository) List(ctx context.Context) ([]*regtokendom.Token, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+regTokenColumns+` FROM agent_registration_tokens ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list registration tokens: %w", err))
	}
	defer rows.Close()
	out := make([]*regtokendom.Token, 0)
	for rows.Next() {
		var t regtokendom.Token
		if err := rows.Scan(&t.ID, &t.Name, &t.TokenHash, &t.CreatedBy, &t.ExpiresAt, &t.UsedAt, &t.ServerID, &t.CreatedAt); err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (r *RegTokenRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM agent_registration_tokens WHERE id = $1`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("delete registration token: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("registration token not found")
	}
	return nil
}
