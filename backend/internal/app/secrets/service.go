// Package secretsapp stores and retrieves plaintext secrets through the
// envelope-encryption layer. Other services depend on this instead of the
// raw repository so plaintext never crosses a boundary unencrypted.
package secretsapp

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	secretdom "github.com/TajBrains/db-manager/backend/internal/domain/secret"
	"github.com/TajBrains/db-manager/backend/internal/platform/crypto"
)

// Service encrypts on write and decrypts on read.
type Service struct {
	repo secretdom.Repository
	enc  *crypto.Encryptor
}

// NewService wires the service.
func NewService(repo secretdom.Repository, enc *crypto.Encryptor) *Service {
	return &Service{repo: repo, enc: enc}
}

// Put encrypts plaintext and upserts it under ref.
func (s *Service) Put(ctx context.Context, ref string, kind secretdom.Kind, plaintext []byte) error {
	env, err := s.enc.Encrypt(plaintext)
	if err != nil {
		return err
	}
	return s.repo.Upsert(ctx, &secretdom.Secret{
		ID:               uuid.New(),
		Ref:              ref,
		Kind:             kind,
		Ciphertext:       env.Ciphertext,
		EncryptedDataKey: env.EncryptedDataKey,
		KeyID:            env.KeyID,
		Nonce:            env.Nonce,
	})
}

// Get returns the decrypted payload for ref.
func (s *Service) Get(ctx context.Context, ref string) ([]byte, error) {
	sec, err := s.repo.GetByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return s.enc.Decrypt(crypto.Envelope{
		Ciphertext:       sec.Ciphertext,
		EncryptedDataKey: sec.EncryptedDataKey,
		Nonce:            sec.Nonce,
		KeyID:            sec.KeyID,
	})
}

// Delete removes the secret stored under ref.
func (s *Service) Delete(ctx context.Context, ref string) error {
	return s.repo.DeleteByRef(ctx, ref)
}

// RotateResult reports the outcome of a key-rotation pass.
type RotateResult struct {
	Total     int
	Rewrapped int
	Skipped   int // already wrapped by the primary key
}

// Rotate re-wraps every secret's data key under the current primary key,
// leaving payload ciphertext untouched. Secrets already at the primary key are
// skipped, so a second run is a no-op. Rotation therefore requires the new key
// to use a new key id (MDCP_ENCRYPTION_KEY_ID).
func (s *Service) Rotate(ctx context.Context) (RotateResult, error) {
	secs, err := s.repo.ListAll(ctx)
	if err != nil {
		return RotateResult{}, err
	}
	res := RotateResult{Total: len(secs)}
	primary := s.enc.KeyID()
	for _, sec := range secs {
		if sec.KeyID == primary {
			res.Skipped++
			continue
		}
		edk, keyID, err := s.enc.Rewrap(crypto.Envelope{
			Ciphertext:       sec.Ciphertext,
			EncryptedDataKey: sec.EncryptedDataKey,
			Nonce:            sec.Nonce,
			KeyID:            sec.KeyID,
		})
		if err != nil {
			return res, fmt.Errorf("rewrap secret %q: %w", sec.Ref, err)
		}
		if err := s.repo.Rewrap(ctx, sec.ID, edk, keyID); err != nil {
			return res, err
		}
		res.Rewrapped++
	}
	return res, nil
}
