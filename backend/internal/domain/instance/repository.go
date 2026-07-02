package instance

import (
	"context"

	"github.com/google/uuid"
)

// ListFilter narrows a List query.
type ListFilter struct {
	ServerID *uuid.UUID
	Kind     *Kind
	Limit    int
	Offset   int
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
	SoftDelete(ctx context.Context, id uuid.UUID) error
}
