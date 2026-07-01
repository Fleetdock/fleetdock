// Package databaseapp holds the application use cases for managed databases.
package databaseapp

import (
	"context"

	"github.com/google/uuid"

	databasedom "github.com/mariadb-cp/db-manager/backend/internal/domain/database"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
)

// CreateInput is the command to register/create a database on an instance.
type CreateInput struct {
	InstanceID string
	Name       string
	Charset    string
	Collation  string
	Labels     map[string]string
	Tags       []string
}

// ListParams are filter + pagination inputs for listing databases.
type ListParams struct {
	InstanceID string
	Status     string
	Search     string
	Limit      int
	Offset     int
}

// ListResult is a page of databases with pagination metadata.
type ListResult struct {
	Items  []*databasedom.Database
	Total  int
	Limit  int
	Offset int
}

const (
	defaultLimit = 20
	maxLimit     = 100
)

// Service implements database use cases.
type Service struct {
	repo databasedom.Repository
}

// NewService wires the service.
func NewService(repo databasedom.Repository) *Service { return &Service{repo: repo} }

// Create validates input and persists a new database record.
func (s *Service) Create(ctx context.Context, in CreateInput) (*databasedom.Database, error) {
	instanceID, err := uuid.Parse(in.InstanceID)
	if err != nil {
		return nil, apperr.Invalid("instance_id", "instance_id must be a valid UUID")
	}
	db, err := databasedom.NewDatabase(instanceID, in.Name, in.Charset, in.Collation, in.Labels, in.Tags)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, db); err != nil {
		return nil, err
	}
	return db, nil
}

// Get returns a database by id.
func (s *Service) Get(ctx context.Context, id string) (*databasedom.Database, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.GetByID(ctx, uid)
}

// List returns a filtered, paginated set of databases.
func (s *Service) List(ctx context.Context, p ListParams) (ListResult, error) {
	limit := clampLimit(p.Limit)
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}
	f := databasedom.ListFilter{Search: p.Search, Limit: limit, Offset: offset}
	if p.InstanceID != "" {
		iid, err := uuid.Parse(p.InstanceID)
		if err != nil {
			return ListResult{}, apperr.Invalid("instance_id", "instance_id must be a valid UUID")
		}
		f.InstanceID = &iid
	}
	if p.Status != "" {
		st := databasedom.Status(p.Status)
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

// Lock marks a database as locked by the given user.
func (s *Service) Lock(ctx context.Context, id string, lockedBy uuid.UUID) (*databasedom.Database, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.Lock(ctx, uid, lockedBy)
}

// Unlock clears the lock on a database.
func (s *Service) Unlock(ctx context.Context, id string) (*databasedom.Database, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.Unlock(ctx, uid)
}

// Delete soft-deletes a database (opening the recovery window).
func (s *Service) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.SoftDelete(ctx, uid)
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
