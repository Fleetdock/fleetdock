package server

import (
	"context"

	"github.com/google/uuid"

	authz "github.com/TajBrains/fleetdock/backend/internal/domain/authz"
)

// ListFilter narrows a List query. Zero values mean "no filter".
type ListFilter struct {
	Status *Status
	Search string // matches name or hostname (substring)
	Tag    string // exact tag membership
	Limit  int
	Offset int
	// Scope, when non-nil, restricts results to the caller's readable scope.
	Scope *authz.ReadSet
}

// Page is a slice of servers plus the total count matching the filter
// (ignoring limit/offset), for pagination.
type Page struct {
	Items []*Server
	Total int
}

// Repository is the persistence port for servers. Adapters live in infra.
type Repository interface {
	Create(ctx context.Context, s *Server) error
	GetByID(ctx context.Context, id uuid.UUID) (*Server, error)
	List(ctx context.Context, f ListFilter) (Page, error)
}
