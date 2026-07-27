package dbcredential

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository is the persistence port for application credentials.
type Repository interface {
	Create(ctx context.Context, c *Credential) error
	GetByID(ctx context.Context, id uuid.UUID) (*Credential, error)
	ListByDatabaseID(ctx context.Context, databaseID uuid.UUID) ([]*Credential, error)
	Revoke(ctx context.Context, id uuid.UUID, at time.Time) error
	ListExpired(ctx context.Context, now time.Time) ([]*Credential, error)
}
