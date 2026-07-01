// Package instanceapp holds the application use cases for instances.
package instanceapp

import (
	"context"

	"github.com/google/uuid"

	instancedom "github.com/mariadb-cp/db-manager/backend/internal/domain/instance"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
)

// RegisterInput is the command to register an instance under a server.
type RegisterInput struct {
	ServerID       string
	Name           string
	MariaDBVersion string
	Port           int
	Labels         map[string]string
	Tags           []string
}

// ListParams are filter + pagination inputs for listing instances.
type ListParams struct {
	ServerID string
	Limit    int
	Offset   int
}

// ListResult is a page of instances with pagination metadata.
type ListResult struct {
	Items  []*instancedom.Instance
	Total  int
	Limit  int
	Offset int
}

const (
	defaultLimit = 20
	maxLimit     = 100
)

// Service implements instance use cases.
type Service struct {
	repo instancedom.Repository
}

// NewService wires the service.
func NewService(repo instancedom.Repository) *Service { return &Service{repo: repo} }

// Register validates input and persists a new instance.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*instancedom.Instance, error) {
	serverID, err := uuid.Parse(in.ServerID)
	if err != nil {
		return nil, apperr.Invalid("server_id", "server_id must be a valid UUID")
	}
	inst, err := instancedom.NewInstance(serverID, in.Name, in.MariaDBVersion, in.Port, in.Labels, in.Tags)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, inst); err != nil {
		return nil, err
	}
	return inst, nil
}

// Get returns an instance by id.
func (s *Service) Get(ctx context.Context, id string) (*instancedom.Instance, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.GetByID(ctx, uid)
}

// List returns a filtered, paginated set of instances.
func (s *Service) List(ctx context.Context, p ListParams) (ListResult, error) {
	limit := clampLimit(p.Limit)
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}
	f := instancedom.ListFilter{Limit: limit, Offset: offset}
	if p.ServerID != "" {
		sid, err := uuid.Parse(p.ServerID)
		if err != nil {
			return ListResult{}, apperr.Invalid("server_id", "server_id must be a valid UUID")
		}
		f.ServerID = &sid
	}
	page, err := s.repo.List(ctx, f)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: page.Items, Total: page.Total, Limit: limit, Offset: offset}, nil
}

func clampLimit(l int) int {
	switch {
	case l <= 0:
		return defaultLimit
	case l > maxLimit:
		return maxLimit
	default:
		return l
	}
}
