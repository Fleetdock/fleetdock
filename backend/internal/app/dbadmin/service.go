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
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/app/dbtarget"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
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

const (
	opTimeout     = 15 * time.Second
	queryTimeout  = 30 * time.Second
	exportTimeout = 5 * time.Minute
)

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

	host, err := dbtarget.Host(ctx, s.servers, inst, "instance_id")
	if err != nil {
		return nil, nil, engine.ConnParams{}, err
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
	return users, apperr.FromEngine(err, "instance")
}

// CreateDBUser creates a database account.
func (s *Service) CreateDBUser(ctx context.Context, instanceID, user, host, password string) error {
	_, admin, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return apperr.FromEngine(admin.CreateDBUser(cctx, conn, user, host, password), "instance")
}

// DropDBUser removes a database account.
func (s *Service) DropDBUser(ctx context.Context, instanceID, user, host string) error {
	_, admin, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return apperr.FromEngine(admin.DropDBUser(cctx, conn, user, host), "instance")
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
	return grants, apperr.FromEngine(err, "instance")
}

// Grant grants schema privileges to an account.
func (s *Service) Grant(ctx context.Context, instanceID, user, host, database string, privileges []string) error {
	_, admin, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return apperr.FromEngine(admin.Grant(cctx, conn, user, host, database, privileges), "instance")
}

// Revoke removes an account's schema privileges.
func (s *Service) Revoke(ctx context.Context, instanceID, user, host, database string) error {
	_, admin, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return apperr.FromEngine(admin.Revoke(cctx, conn, user, host, database), "instance")
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
	return grants, apperr.FromEngine(err, "instance")
}

// GrantOnDatabase grants privileges on this database to an account.
func (s *Service) GrantOnDatabase(ctx context.Context, databaseID, user, host string, privileges []string) error {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return apperr.FromEngine(admin.Grant(cctx, conn, user, host, db.Name, privileges), "instance")
}

// RevokeOnDatabase revokes an account's privileges on this database.
func (s *Service) RevokeOnDatabase(ctx context.Context, databaseID, user, host string) error {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return apperr.FromEngine(admin.Revoke(cctx, conn, user, host, db.Name), "instance")
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
	return tables, apperr.FromEngine(err, "instance")
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
	return page, apperr.FromEngine(err, "instance")
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
	return users, apperr.FromEngine(err, "instance")
}

// TableSchema returns a table's columns, indexes and CREATE DDL.
func (s *Service) TableSchema(ctx context.Context, databaseID, table string) (*engine.TableSchema, error) {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	schema, err := admin.TableSchema(cctx, conn, db.Name, table)
	return schema, apperr.FromEngine(err, "instance")
}

// Query runs an ad-hoc console statement against the database. Writes are only
// permitted when allowWrite is true (the handler derives this from the caller's
// database:write permission).
func (s *Service) Query(ctx context.Context, databaseID, sql string, limit int, allowWrite bool) (*engine.QueryResult, error) {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	res, err := admin.Query(cctx, conn, db.Name, sql, limit, allowWrite)
	return res, apperr.FromEngine(err, "instance")
}

// ExportTableCSV streams a whole table to w as CSV. onStart is invoked once the
// result set opens successfully, before any bytes are written.
func (s *Service) ExportTableCSV(ctx context.Context, databaseID, table string, w io.Writer, onStart func()) (int64, error) {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return 0, err
	}
	cctx, cancel := context.WithTimeout(ctx, exportTimeout)
	defer cancel()
	n, err := admin.ExportCSV(cctx, conn, db.Name, table, "", w, onStart)
	return n, apperr.FromEngine(err, "instance")
}

// ExportQueryCSV streams a read-only query's result to w as CSV.
func (s *Service) ExportQueryCSV(ctx context.Context, databaseID, sql string, w io.Writer, onStart func()) (int64, error) {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return 0, err
	}
	cctx, cancel := context.WithTimeout(ctx, exportTimeout)
	defer cancel()
	n, err := admin.ExportCSV(cctx, conn, db.Name, "", sql, w, onStart)
	return n, apperr.FromEngine(err, "instance")
}
