// Package instanceapp holds the application use cases for instances —
// managed (on registered servers) and external (existing databases anywhere,
// e.g. instances already running under Dokploy).
package instanceapp

import (
	"context"
	"time"

	"github.com/google/uuid"

	operationapp "github.com/mariadb-cp/db-manager/backend/internal/app/operation"
	databasedom "github.com/mariadb-cp/db-manager/backend/internal/domain/database"
	instancedom "github.com/mariadb-cp/db-manager/backend/internal/domain/instance"
	jobdom "github.com/mariadb-cp/db-manager/backend/internal/domain/job"
	secretdom "github.com/mariadb-cp/db-manager/backend/internal/domain/secret"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/engine"
)

// Secrets is the secret store surface this service needs.
type Secrets interface {
	Put(ctx context.Context, ref string, kind secretdom.Kind, plaintext []byte) error
	Get(ctx context.Context, ref string) ([]byte, error)
	Delete(ctx context.Context, ref string) error
}

// RegisterInput is the command to register an instance.
type RegisterInput struct {
	// Kind: "managed" (default) requires ServerID; "external" requires Host.
	Kind          string
	ServerID      string
	Host          string
	Name          string
	Engine        string
	EngineVersion string
	Port          int
	Username      string
	Password      string // write-only; stored encrypted
	Labels        map[string]string
	Tags          []string
}

// ListParams are filter + pagination inputs for listing instances.
type ListParams struct {
	ServerID string
	Kind     string
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
	repo      instancedom.Repository
	databases databasedom.Repository
	secrets   Secrets
	ops       *operationapp.Service
}

// NewService wires the service.
func NewService(repo instancedom.Repository, databases databasedom.Repository, secrets Secrets, ops *operationapp.Service) *Service {
	return &Service{repo: repo, databases: databases, secrets: secrets, ops: ops}
}

// Register validates input and persists a new instance (managed or external).
func (s *Service) Register(ctx context.Context, in RegisterInput) (*instancedom.Instance, error) {
	var username *string
	if in.Username != "" {
		username = &in.Username
	}
	if in.Password != "" && in.Username == "" {
		return nil, apperr.Invalid("username", "username is required when a password is provided")
	}

	var (
		inst *instancedom.Instance
		err  error
	)
	switch in.Kind {
	case "", string(instancedom.KindManaged):
		serverID, perr := uuid.Parse(in.ServerID)
		if perr != nil {
			return nil, apperr.Invalid("server_id", "server_id must be a valid UUID")
		}
		inst, err = instancedom.NewManaged(serverID, in.Name, instancedom.Engine(in.Engine), in.EngineVersion, in.Port, username, in.Labels, in.Tags)
	case string(instancedom.KindExternal):
		inst, err = instancedom.NewExternal(in.Name, instancedom.Engine(in.Engine), in.EngineVersion, in.Host, in.Port, username, in.Labels, in.Tags)
	default:
		return nil, apperr.Invalid("kind", "kind must be managed or external")
	}
	if err != nil {
		return nil, err
	}

	if in.Password != "" {
		ref := "instance/" + inst.ID.String() + "/root"
		if err := s.secrets.Put(ctx, ref, secretdom.KindMariaDBRoot, []byte(in.Password)); err != nil {
			return nil, err
		}
		inst.RootSecretRef = &ref
	}

	if err := s.repo.Create(ctx, inst); err != nil {
		if inst.RootSecretRef != nil {
			_ = s.secrets.Delete(ctx, *inst.RootSecretRef)
		}
		return nil, err
	}
	if inst.RootSecretRef != nil {
		if err := s.repo.SetRootSecretRef(ctx, inst.ID, *inst.RootSecretRef); err != nil {
			return nil, err
		}
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

// Delete soft-deletes an instance record (does not touch the actual server).
func (s *Service) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.SoftDelete(ctx, uid)
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
	if p.Kind != "" {
		k := instancedom.Kind(p.Kind)
		f.Kind = &k
	}
	page, err := s.repo.List(ctx, f)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: page.Items, Total: page.Total, Limit: limit, Offset: offset}, nil
}

// TestConnectionResult reports connectivity for an instance.
type TestConnectionResult struct {
	Mode        string // sync | async
	OK          bool
	Version     string
	Error       string
	OperationID string
}

// TestConnection checks connectivity: synchronously for external instances,
// via an agent operation for managed ones.
func (s *Service) TestConnection(ctx context.Context, id string, createdBy *uuid.UUID) (*TestConnectionResult, error) {
	inst, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !inst.HasCredentials() {
		return nil, apperr.Invalid("id", "instance has no admin credentials configured")
	}

	if inst.Kind == instancedom.KindExternal {
		conn, err := s.connParams(ctx, inst)
		if err != nil {
			return nil, err
		}
		eng, err := engine.For(string(inst.Engine))
		if err != nil {
			return nil, apperr.Invalid("engine", err.Error())
		}
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		version, perr := eng.Ping(cctx, conn)
		res := &TestConnectionResult{Mode: "sync", OK: perr == nil, Version: version}
		if perr != nil {
			res.Error = perr.Error()
		}
		return res, nil
	}

	job, err := s.ops.Create(ctx, jobdom.TypeTestConnection, "instance", &inst.ID, inst.ServerID,
		operationapp.Params{InstanceID: inst.ID.String()}, createdBy)
	if err != nil {
		return nil, err
	}
	return &TestConnectionResult{Mode: "async", OperationID: job.ID.String()}, nil
}

// ImportResult reports the outcome of a database import.
type ImportResult struct {
	Mode        string // sync | async
	Imported    int
	OperationID string
}

// ImportDatabases discovers existing databases on the instance and registers
// them: synchronously for external instances, via an agent operation for
// managed ones.
func (s *Service) ImportDatabases(ctx context.Context, id string, createdBy *uuid.UUID) (*ImportResult, error) {
	inst, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !inst.HasCredentials() {
		return nil, apperr.Invalid("id", "instance has no admin credentials configured")
	}

	if inst.Kind == instancedom.KindExternal {
		conn, err := s.connParams(ctx, inst)
		if err != nil {
			return nil, err
		}
		eng, err := engine.For(string(inst.Engine))
		if err != nil {
			return nil, apperr.Invalid("engine", err.Error())
		}
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		dbs, err := eng.ListDatabases(cctx, conn)
		if err != nil {
			return nil, apperr.Invalid("id", "could not list databases: "+err.Error())
		}
		imported := 0
		for _, d := range dbs {
			db, derr := databasedom.NewDatabase(inst.ID, d.Name, d.Charset, d.Collation,
				map[string]string{"imported": "true"}, nil)
			if derr != nil {
				continue
			}
			if cerr := s.databases.Create(ctx, db); cerr == nil {
				imported++
			}
		}
		return &ImportResult{Mode: "sync", Imported: imported}, nil
	}

	job, err := s.ops.Create(ctx, jobdom.TypeImportDatabases, "instance", &inst.ID, inst.ServerID,
		operationapp.Params{InstanceID: inst.ID.String()}, createdBy)
	if err != nil {
		return nil, err
	}
	return &ImportResult{Mode: "async", OperationID: job.ID.String()}, nil
}

func (s *Service) connParams(ctx context.Context, inst *instancedom.Instance) (engine.ConnParams, error) {
	host := "127.0.0.1"
	if inst.Host != nil {
		host = *inst.Host
	}
	conn := engine.ConnParams{Host: host, Port: inst.Port}
	if inst.Username != nil {
		conn.User = *inst.Username
	}
	if inst.RootSecretRef != nil {
		pw, err := s.secrets.Get(ctx, *inst.RootSecretRef)
		if err != nil {
			return conn, err
		}
		conn.Password = string(pw)
	}
	return conn, nil
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
