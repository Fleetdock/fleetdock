package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	secretdom "github.com/TajBrains/fleetdock/backend/internal/domain/secret"
	"github.com/TajBrains/fleetdock/backend/internal/platform/apperr"
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

// ListAll returns every stored secret (used by key rotation).
func (r *SecretRepository) ListAll(ctx context.Context) ([]*secretdom.Secret, error) {
	const q = `
		SELECT id, ref, kind, ciphertext, encrypted_data_key, key_id, nonce, created_at, rotated_at
		FROM secrets ORDER BY created_at`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list secrets: %w", err))
	}
	defer rows.Close()
	out := make([]*secretdom.Secret, 0)
	for rows.Next() {
		var (
			s    secretdom.Secret
			kind string
		)
		if err := rows.Scan(
			&s.ID, &s.Ref, &kind, &s.Ciphertext, &s.EncryptedDataKey, &s.KeyID, &s.Nonce, &s.CreatedAt, &s.RotatedAt,
		); err != nil {
			return nil, apperr.Internal(err)
		}
		s.Kind = secretdom.Kind(kind)
		out = append(out, &s)
	}
	return out, rows.Err()
}

// Rewrap replaces a secret's wrapped data key and key id (payload untouched).
func (r *SecretRepository) Rewrap(ctx context.Context, id uuid.UUID, encryptedDataKey []byte, keyID string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE secrets SET encrypted_data_key = $2, key_id = $3, rotated_at = now() WHERE id = $1`,
		id, encryptedDataKey, keyID)
	if err != nil {
		return apperr.Internal(fmt.Errorf("rewrap secret: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("secret not found")
	}
	return nil
}
