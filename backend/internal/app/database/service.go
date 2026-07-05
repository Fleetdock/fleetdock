// Package databaseapp holds the application use cases for managed databases.
package databaseapp

import (
	"context"

	"github.com/google/uuid"

	operationapp "github.com/mariadb-cp/db-manager/backend/internal/app/operation"
	databasedom "github.com/mariadb-cp/db-manager/backend/internal/domain/database"
	instancedom "github.com/mariadb-cp/db-manager/backend/internal/domain/instance"
	jobdom "github.com/mariadb-cp/db-manager/backend/internal/domain/job"
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
	repo      databasedom.Repository
	instances instancedom.Repository
	ops       *operationapp.Service
}

// NewService wires the service.
func NewService(repo databasedom.Repository, instances instancedom.Repository, ops *operationapp.Service) *Service {
	return &Service{repo: repo, instances: instances, ops: ops}
}

// Create validates input and persists a new database record. When the
// instance has admin credentials, the database is physically created through
// an operation (agent for managed instances, control plane for external
// ones); otherwise it is a metadata-only registration.
func (s *Service) Create(ctx context.Context, in CreateInput, createdBy *uuid.UUID) (*databasedom.Database, error) {
	instanceID, err := uuid.Parse(in.InstanceID)
	if err != nil {
		return nil, apperr.Invalid("instance_id", "instance_id must be a valid UUID")
	}
	inst, err := s.instances.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	db, err := databasedom.NewDatabase(instanceID, in.Name, in.Charset, in.Collation, in.Labels, in.Tags)
	if err != nil {
		return nil, err
	}
	provision := inst.HasCredentials()
	if provision {
		db.Status = databasedom.StatusCreating
	}
	if err := s.repo.Create(ctx, db); err != nil {
		return nil, err
	}
	if provision {
		serverID := inst.ServerID
		if inst.Kind == instancedom.KindExternal {
			serverID = nil
		}
		if _, err := s.ops.Create(ctx, jobdom.TypeCreateDatabase, "database", &db.ID, serverID,
			operationapp.Params{
				InstanceID: inst.ID.String(),
				DatabaseID: db.ID.String(),
				Database:   db.Name,
				Charset:    db.Charset,
				Collation:  db.Collation,
			}, createdBy); err != nil {
			return nil, err
		}
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

// Delete soft-deletes a database. When dropPhysical is true and the instance
// has admin credentials, a delete_database operation is enqueued to run
// DROP DATABASE on the instance before the control-plane record is removed.
func (s *Service) Delete(ctx context.Context, id string, dropPhysical bool, createdBy *uuid.UUID) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	db, err := s.repo.GetByID(ctx, uid)
	if err != nil {
		return err
	}
	inst, err := s.instances.GetByID(ctx, db.InstanceID)
	if err != nil {
		return err
	}
	if dropPhysical {
		if !inst.HasCredentials() {
			return apperr.Invalid("drop", "instance has no admin credentials; cannot drop database on server")
		}
		serverID := inst.ServerID
		if inst.Kind == instancedom.KindExternal {
			serverID = nil
		}
		if _, err := s.ops.Create(ctx, jobdom.TypeDeleteDatabase, "database", &db.ID, serverID,
			operationapp.Params{
				InstanceID: inst.ID.String(),
				DatabaseID: db.ID.String(),
				Database:   db.Name,
			}, createdBy); err != nil {
			return err
		}
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
