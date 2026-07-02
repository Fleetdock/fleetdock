// Package regtoken is the domain model for one-time agent registration
// tokens: a token is generated in the dashboard, embedded in the install
// command, and consumed exactly once when the agent enrolls.
package regtoken

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Token is a single-use agent enrollment credential (hash stored, raw shown once).
type Token struct {
	ID        uuid.UUID
	Name      string
	TokenHash string
	CreatedBy *uuid.UUID
	ExpiresAt time.Time
	UsedAt    *time.Time
	ServerID  *uuid.UUID
	CreatedAt time.Time
}

// Repository is the persistence port for registration tokens.
type Repository interface {
	Create(ctx context.Context, t *Token) error
	GetByHash(ctx context.Context, hash string) (*Token, error)
	// MarkUsed consumes the token atomically; it fails if already used.
	MarkUsed(ctx context.Context, id, serverID uuid.UUID) error
	List(ctx context.Context) ([]*Token, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
