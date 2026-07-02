package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	secretdom "github.com/mariadb-cp/db-manager/backend/internal/domain/secret"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
)

// SecretRepository is the Postgres adapter for secretdom.Repository.
type SecretRepository struct {
	pool *pgxpool.Pool
}

// NewSecretRepository builds a secret repository.
func NewSecretRepository(pool *pgxpool.Pool) *SecretRepository { return &SecretRepository{pool: pool} }

var _ secretdom.Repository = (*SecretRepository)(nil)

func (r *SecretRepository) Upsert(ctx context.Context, s *secretdom.Secret) error {
	const q = `
		INSERT INTO secrets (id, ref, kind, ciphertext, encrypted_data_key, key_id, nonce)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (ref) DO UPDATE SET
			ciphertext = EXCLUDED.ciphertext,
			encrypted_data_key = EXCLUDED.encrypted_data_key,
			key_id = EXCLUDED.key_id,
			nonce = EXCLUDED.nonce,
			rotated_at = now()`
	_, err := r.pool.Exec(ctx, q, s.ID, s.Ref, string(s.Kind), s.Ciphertext, s.EncryptedDataKey, s.KeyID, s.Nonce)
	if err != nil {
		return apperr.Internal(fmt.Errorf("upsert secret: %w", err))
	}
	return nil
}

func (r *SecretRepository) GetByRef(ctx context.Context, ref string) (*secretdom.Secret, error) {
	const q = `
		SELECT id, ref, kind, ciphertext, encrypted_data_key, key_id, nonce, created_at, rotated_at
		FROM secrets WHERE ref = $1`
	var (
		s    secretdom.Secret
		kind string
	)
	err := r.pool.QueryRow(ctx, q, ref).Scan(
		&s.ID, &s.Ref, &kind, &s.Ciphertext, &s.EncryptedDataKey, &s.KeyID, &s.Nonce, &s.CreatedAt, &s.RotatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("secret not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get secret: %w", err))
	}
	s.Kind = secretdom.Kind(kind)
	return &s, nil
}

func (r *SecretRepository) DeleteByRef(ctx context.Context, ref string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM secrets WHERE ref = $1`, ref)
	if err != nil {
		return apperr.Internal(fmt.Errorf("delete secret: %w", err))
	}
	return nil
}
