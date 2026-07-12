package instance

import (
	"context"

	"github.com/google/uuid"

	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
)

// ListFilter narrows a List query.
type ListFilter struct {
	ServerID *uuid.UUID
	Kind     *Kind
	Limit    int
	Offset   int
	// Scope, when non-nil, restricts results to the caller's readable scope.
	Scope *authz.ReadSet
}

// Page is a slice of instances plus the total matching count.
type Page struct {
	Items []*Instance
	Total int
}

// Repository is the persistence port for instances.
type Repository interface {
	Create(ctx context.Context, in *Instance) error
	GetByID(ctx context.Context, id uuid.UUID) (*Instance, error)
	List(ctx context.Context, f ListFilter) (Page, error)
	// SetRootSecretRef links the encrypted admin credential to the instance.
	SetRootSecretRef(ctx context.Context, id uuid.UUID, ref string) error
	// SetStatus transitions the instance lifecycle status.
	SetStatus(ctx context.Context, id uuid.UUID, status Status) error
	// SetContainerID records the Docker container id of a provisioned instance.
	SetContainerID(ctx context.Context, id uuid.UUID, containerID string) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
}
