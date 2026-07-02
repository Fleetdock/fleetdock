// Package dbadminapp exposes live database administration (database users,
// grants, table browsing) executed synchronously by the control plane.
//
// Connectivity: external instances are reached at their host; managed
// instances at their server's address (or hostname). This requires the
// database port to be reachable from the control plane — the agent job
// channel is not interactive enough for browsing.
package dbadminapp

import (
	"context"
	"time"

	"github.com/google/uuid"

	databasedom "github.com/mariadb-cp/db-manager/backend/internal/domain/database"
	instancedom "github.com/mariadb-cp/db-manager/backend/internal/domain/instance"
	serverdom "github.com/mariadb-cp/db-manager/backend/internal/domain/server"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/engine"
)

// Secrets is the secret store surface this service needs.
type Secrets interface {
	Get(ctx context.Context, ref string) ([]byte, error)
}

// Service implements live DB administration use cases.
type Service struct {
	instances instancedom.Repository
	databases databasedom.Repository
	servers   serverdom.Repository
	secrets   Secrets
}

// NewService wires the service.
func NewService(instances instancedom.Repository, databases databasedom.Repository,
	servers serverdom.Repository, secrets Secrets) *Service {
	return &Service{instances: instances, databases: databases, servers: servers, secrets: secrets}
}

const opTimeout = 15 * time.Second

// target resolves the instance, admin surface and connection parameters.
func (s *Service) target(ctx context.Context, instanceID string) (*instancedom.Instance, engine.Admin, engine.ConnParams, error) {
	iid, err := uuid.Parse(instanceID)
	if err != nil {
		return nil, nil, engine.ConnParams{}, apperr.Invalid("instance_id", "instance_id must be a valid UUID")
	}
	inst, err := s.instances.GetByID(ctx, iid)
	if err != nil {
		return nil, nil, engine.ConnParams{}, err
	}
	if !inst.HasCredentials() {
		return nil, nil, engine.ConnParams{}, apperr.Invalid("instance_id",
			"instance has no admin credentials; add a username/password to enable administration")
	}
	admin, err := engine.AdminFor(string(inst.Engine))
	if err != nil {
		return nil, nil, engine.ConnParams{}, apperr.Invalid("engine", err.Error())
	}

	host := ""
	switch {
	case inst.Kind == instancedom.KindExternal && inst.Host != nil:
		host = *inst.Host
	case inst.ServerID != nil:
		srv, err := s.servers.GetByID(ctx, *inst.ServerID)
		if err != nil {
			return nil, nil, engine.ConnParams{}, err
		}
		if srv.Address != nil && *srv.Address != "" {
			host = *srv.Address
		} else {
			host = srv.Hostname
		}
	}
	if host == "" {
		return nil, nil, engine.ConnParams{}, apperr.Invalid("instance_id", "cannot determine a reachable host for this instance")
	}

	conn := engine.ConnParams{Host: host, Port: inst.Port}
	if inst.Username != nil {
		conn.User = *inst.Username
	}
	pw, err := s.secrets.Get(ctx, *inst.RootSecretRef)
	if err != nil {
		return nil, nil, engine.ConnParams{}, apperr.Internal(err)
	}
	conn.Password = string(pw)
	return inst, admin, conn, nil
}

// databaseTarget resolves a database plus its instance's admin surface.
func (s *Service) databaseTarget(ctx context.Context, databaseID string) (*databasedom.Database, engine.Admin, engine.ConnParams, error) {
	did, err := uuid.Parse(databaseID)
	if err != nil {
		return nil, nil, engine.ConnParams{}, apperr.Invalid("id", "id must be a valid UUID")
	}
	db, err := s.databases.GetByID(ctx, did)
	if err != nil {
		return nil, nil, engine.ConnParams{}, err
	}
	_, admin, conn, err := s.target(ctx, db.InstanceID.String())
	if err != nil {
		return nil, nil, engine.ConnParams{}, err
	}
	return db, admin, conn, nil
}

func wrap(err error) error {
	if err == nil {
		return nil
	}
	if apperr.KindOf(err) != apperr.KindInternal {
		return err
	}
	// Engine/network errors are user-actionable here (bad grants, unreachable
	// host, ...), so surface the message instead of a blank 500.
	return apperr.Invalid("instance", err.Error())
}

// ---- Instance-level: users & grants ----

// ListDBUsers lists database accounts on an instance.
func (s *Service) ListDBUsers(ctx context.Context, instanceID string) ([]engine.DBUser, error) {
	_, admin, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	users, err := admin.ListDBUsers(cctx, conn)
	return users, wrap(err)
}

// CreateDBUser creates a database account.
func (s *Service) CreateDBUser(ctx context.Context, instanceID, user, host, password string) error {
	_, admin, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return wrap(admin.CreateDBUser(cctx, conn, user, host, password))
}

// DropDBUser removes a database account.
func (s *Service) DropDBUser(ctx context.Context, instanceID, user, host string) error {
	_, admin, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return wrap(admin.DropDBUser(cctx, conn, user, host))
}

// UserGrants returns SHOW GRANTS output for an account.
func (s *Service) UserGrants(ctx context.Context, instanceID, user, host string) ([]string, error) {
	_, admin, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	grants, err := admin.UserGrants(cctx, conn, user, host)
	return grants, wrap(err)
}

// Grant grants schema privileges to an account.
func (s *Service) Grant(ctx context.Context, instanceID, user, host, database string, privileges []string) error {
	_, admin, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return wrap(admin.Grant(cctx, conn, user, host, database, privileges))
}

// Revoke removes an account's schema privileges.
func (s *Service) Revoke(ctx context.Context, instanceID, user, host, database string) error {
	_, admin, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return wrap(admin.Revoke(cctx, conn, user, host, database))
}

// ---- Database-level: grants, tables, data ----

// SchemaGrants lists per-account privileges on a database.
func (s *Service) SchemaGrants(ctx context.Context, databaseID string) ([]engine.SchemaGrant, error) {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	grants, err := admin.SchemaGrants(cctx, conn, db.Name)
	return grants, wrap(err)
}

// GrantOnDatabase grants privileges on this database to an account.
func (s *Service) GrantOnDatabase(ctx context.Context, databaseID, user, host string, privileges []string) error {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return wrap(admin.Grant(cctx, conn, user, host, db.Name, privileges))
}

// RevokeOnDatabase revokes an account's privileges on this database.
func (s *Service) RevokeOnDatabase(ctx context.Context, databaseID, user, host string) error {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return wrap(admin.Revoke(cctx, conn, user, host, db.Name))
}

// ListTables lists tables in a database.
func (s *Service) ListTables(ctx context.Context, databaseID string) ([]engine.TableInfo, error) {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	tables, err := admin.ListTables(cctx, conn, db.Name)
	return tables, wrap(err)
}

// TableRows returns one page of table data.
func (s *Service) TableRows(ctx context.Context, databaseID, table string, limit, offset int) (*engine.RowsPage, error) {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	page, err := admin.TableRows(cctx, conn, db.Name, table, limit, offset)
	return page, wrap(err)
}

// ListDBUsersForDatabase lists instance accounts (for the grant form on the
// database detail page).
func (s *Service) ListDBUsersForDatabase(ctx context.Context, databaseID string) ([]engine.DBUser, error) {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	_ = db
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	users, err := admin.ListDBUsers(cctx, conn)
	return users, wrap(err)
}
