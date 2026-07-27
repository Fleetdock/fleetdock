// Package secret is the domain model for envelope-encrypted secret material.
package secret

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Kind classifies what a secret protects.
type Kind string

const (
	KindMariaDBRoot  Kind = "mariadb_root"
	KindMariaDBUser  Kind = "mariadb_user"
	KindPostgresUser Kind = "postgres_user"
	KindS3Credential Kind = "s3_credential"
	KindAgentEnroll  Kind = "agent_enrollment"
	KindOther        Kind = "other"
)

// Secret is an encrypted payload addressed by a logical ref.
type Secret struct {
	ID               uuid.UUID
	Ref              string
	Kind             Kind
	Ciphertext       []byte
	EncryptedDataKey []byte
	KeyID            string
	Nonce            []byte
	CreatedAt        time.Time
	RotatedAt        *time.Time
}

// Repository is the persistence port for secrets.
type Repository interface {
	Upsert(ctx context.Context, s *Secret) error
	GetByRef(ctx context.Context, ref string) (*Secret, error)
	DeleteByRef(ctx context.Context, ref string) error
	// ListAll returns every stored secret (used by key rotation).
	ListAll(ctx context.Context) ([]*Secret, error)
	// Rewrap replaces a secret's wrapped data key and key id after re-wrapping
	// under a new master key (payload ciphertext is unchanged).
	Rewrap(ctx context.Context, id uuid.UUID, encryptedDataKey []byte, keyID string) error
}
