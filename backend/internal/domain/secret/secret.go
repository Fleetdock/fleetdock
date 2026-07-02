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
}
