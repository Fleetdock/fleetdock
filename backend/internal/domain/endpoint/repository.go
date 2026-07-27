package endpoint

import (
	"context"

	"github.com/google/uuid"
)

// Repository is the persistence port for database endpoints.
type Repository interface {
	// CreateWithPort allocates the lowest free port in [start,end] and inserts
	// the endpoint in one transaction, setting e.ExternalPort on success.
	// Allocating and inserting separately races: two concurrent enables would
	// pick the same port and one would fail on the uniqueness constraint.
	CreateWithPort(ctx context.Context, e *Endpoint, start, end int) error
	GetPublicByDatabaseID(ctx context.Context, databaseID uuid.UUID) (*Endpoint, error)
	// ListRoutable returns public endpoints that belong in the gateway config
	// (pending, active, and error).
	ListRoutable(ctx context.Context) ([]*Endpoint, error)
	// ListDisabling returns public endpoints awaiting removal from the config.
	ListDisabling(ctx context.Context) ([]*Endpoint, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status Status, lastError *string) error
	UpdateBackend(ctx context.Context, id uuid.UUID, internalHost string, internalPort int) error
	UpdateTLSStatus(ctx context.Context, id uuid.UUID, tlsStatus TLSStatus) error
	UpdateAllowedCIDRs(ctx context.Context, id uuid.UUID, cidrs []string) error
	// TransferDatabase moves endpoints from one database to another (moves).
	TransferDatabase(ctx context.Context, fromDatabaseID, toDatabaseID uuid.UUID) error
	DisablePublic(ctx context.Context, databaseID uuid.UUID) error
}
