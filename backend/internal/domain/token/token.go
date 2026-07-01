// Package token is the domain model for API tokens.
package token

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Token is an API token belonging to a user. The raw secret is never stored;
// only its hash. Scopes is a subset of the permission catalog.
type Token struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Name       string
	Prefix     string
	Scopes     []string
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// Lookup is the result of resolving a presented token hash.
type Lookup struct {
	UserID uuid.UUID
	Scopes []string
}

// Repository is the persistence port for API tokens.
type Repository interface {
	Create(ctx context.Context, t *Token, hash string) error
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*Token, error)
	Revoke(ctx context.Context, userID, id uuid.UUID) error
	// ResolveHash returns the owner + scopes for a valid (non-revoked, unexpired) token.
	ResolveHash(ctx context.Context, hash string) (Lookup, error)
}
