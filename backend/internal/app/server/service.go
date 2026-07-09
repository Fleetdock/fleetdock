// Package serverapp holds the application use cases for managing servers.
// It orchestrates the domain and the Repository port; it knows nothing about
// HTTP or SQL.
package serverapp

import (
	"context"

	"github.com/google/uuid"

	authz "github.com/TajBrains/db-manager/backend/internal/domain/authz"
	serverdom "github.com/TajBrains/db-manager/backend/internal/domain/server"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// RegisterInput is the command to register a new server.
type RegisterInput struct {
	Name     string
	Hostname string
	Address  *string
	OS       *string
	Labels   map[string]string
	Tags     []string
}

// ListParams are the filter + pagination inputs for listing servers.
type ListParams struct {
	Status string
	Search string
	Tag    string
	Limit  int
	Offset int
	Scope  *authz.ReadSet
}

// ListResult is a page of servers with pagination metadata.
type ListResult struct {
	Items  []*serverdom.Server
	Total  int
	Limit  int
	Offset int
}

const (
	defaultLimit = 20
	maxLimit     = 100
)

// Service implements the server-management use cases.
type Service struct {
	repo serverdom.Repository
}

// NewService wires the service to a repository implementation.
func NewService(repo serverdom.Repository) *Service { return &Service{repo: repo} }

// Register validates input, builds the aggregate and persists it.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*serverdom.Server, error) {
	srv, err := serverdom.NewServer(in.Name, in.Hostname, in.Address, in.OS, in.Labels, in.Tags)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, srv); err != nil {
		return nil, err
	}
	return srv, nil
}

// Get returns a single server by id.
func (s *Service) Get(ctx context.Context, id string) (*serverdom.Server, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.GetByID(ctx, uid)
}

// List returns a filtered, paginated set of servers with clamped bounds.
func (s *Service) List(ctx context.Context, p ListParams) (ListResult, error) {
	limit := p.Limit
	switch {
	case limit <= 0:
		limit = defaultLimit
	case limit > maxLimit:
		limit = maxLimit
	}
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}

	f := serverdom.ListFilter{Search: p.Search, Tag: p.Tag, Limit: limit, Offset: offset, Scope: p.Scope}
	if p.Status != "" {
		st := serverdom.Status(p.Status)
		if !st.Valid() {
			return ListResult{}, apperr.Invalid("status", "unknown status filter")
		}
		f.Status = &st
	}

	page, err := s.repo.List(ctx, f)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: page.Items, Total: page.Total, Limit: limit, Offset: offset}, nil
}
