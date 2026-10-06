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

// Credentials is the admin login an instance uses for SQL operations. Both
// fields nil means "no credentials" — the instance becomes metadata-only.
type Credentials struct {
	Username      *string
	RootSecretRef *string
}

// UpdateFields is a partial update of an instance's mutable metadata. Nil
// fields are left untouched; a non-nil Credentials replaces both the admin
// username and the secret reference together, since they are only meaningful
// as a pair.
type UpdateFields struct {
	Name        *string
	Host        *string
	Port        *int
	TLSMode     *string
	Credentials *Credentials
}

// Repository is the persistence port for instances.
type Repository interface {
	Create(ctx context.Context, in *Instance) error
	GetByID(ctx context.Context, id uuid.UUID) (*Instance, error)
	List(ctx context.Context, f ListFilter) (Page, error)
	// Update applies a partial change to mutable instance metadata.
	Update(ctx context.Context, id uuid.UUID, f UpdateFields) error
	// SetRootSecretRef links the encrypted admin credential to the instance.
	SetRootSecretRef(ctx context.Context, id uuid.UUID, ref string) error
	// SetStatus transitions the instance lifecycle status.
	SetStatus(ctx context.Context, id uuid.UUID, status Status) error
	// SetContainerID records the Docker container id of a provisioned instance.
	SetContainerID(ctx context.Context, id uuid.UUID, containerID string) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	// SetHealth records the latest probe result.
	SetHealth(ctx context.Context, id uuid.UUID, h Health) error
}
