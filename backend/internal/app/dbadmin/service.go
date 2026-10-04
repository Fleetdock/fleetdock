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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	dbaccessapp "github.com/Fleetdock/fleetdock/backend/internal/app/dbaccess"
	"github.com/Fleetdock/fleetdock/backend/internal/app/dbtarget"
	consoledom "github.com/Fleetdock/fleetdock/backend/internal/domain/console"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	dbaccessdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbaccess"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// Secrets is the secret store surface this service needs.
type Secrets interface {
	Get(ctx context.Context, ref string) ([]byte, error)
}

// Access resolves the least-privilege role interactive requests run as.
type Access interface {
	Conn(ctx context.Context, t dbaccessapp.Target, mode dbaccessdom.Mode) (engine.ConnParams, error)
	Reapply(ctx context.Context, t dbaccessapp.Target, mode dbaccessdom.Mode) error
}

// Service implements live DB administration use cases.
//
// Two kinds of connection are used. Instance-level administration (accounts,
// grants) runs as the instance admin. Everything that executes or reads user
// data for one database — the console, table browsing, exports — runs as that
// database's Fleetdock-managed console role (see dbaccessapp), so the engine
// confines it to that database.
type Service struct {
	instances instancedom.Repository
	databases databasedom.Repository
	servers   serverdom.Repository
	secrets   Secrets
	access    Access
	history   History

	runningMu sync.Mutex
	running   map[string]*runningQuery
}

// NewService wires the service.
func NewService(instances instancedom.Repository, databases databasedom.Repository,
	servers serverdom.Repository, secrets Secrets, access Access) *Service {
	return &Service{instances: instances, databases: databases, servers: servers, secrets: secrets, access: access}
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

	conn := engine.ConnParams{Host: host, Port: inst.Port, TLSMode: inst.TLSModeOrDefault()}
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

// SystemDatabaseInstance reports whether a database is engine-owned (mysql,
// sys, postgres) and, if so, its instance. Those hold account definitions and
// password hashes, so interactive access to them requires instance-level
// rights, not just database access.
func (s *Service) SystemDatabaseInstance(ctx context.Context, databaseID string) (uuid.UUID, bool, error) {
	did, err := uuid.Parse(databaseID)
	if err != nil {
		return uuid.Nil, false, apperr.Invalid("id", "id must be a valid UUID")
	}
	db, err := s.databases.GetByID(ctx, did)
	if err != nil {
		return uuid.Nil, false, err
	}
	return db.InstanceID, db.System, nil
}

// scoped runs fn as the database's console role. Write mode additionally
// requires the database to be active (not locked, migrating or deleting). If
// the role is refused for lack of privileges — on PostgreSQL grants do not
// cover objects created later by other roles — its grants are re-applied and
// fn retried once.
func (s *Service) scoped(ctx context.Context, databaseID string, mode dbaccessdom.Mode, timeout time.Duration,
	fn func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error) error {
	did, err := uuid.Parse(databaseID)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	db, err := s.databases.GetByID(ctx, did)
	if err != nil {
		return err
	}
	if mode == dbaccessdom.ModeWrite && db.Status != databasedom.StatusActive {
		return apperr.Conflict(fmt.Sprintf("database is %s; writes are not allowed until it is active again", db.Status))
	}
	inst, admin, root, err := s.target(ctx, db.InstanceID.String())
	if err != nil {
		return err
	}
	t := dbaccessapp.Target{Instance: inst, Database: db, Admin: admin, Root: root}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := s.access.Conn(cctx, t, mode)
	if err != nil {
		return err
	}
	conn.StatementTimeout = timeout

	err = fn(cctx, db, admin, conn)
	if err != nil && engine.IsPermissionDenied(err) {
		if rerr := s.access.Reapply(cctx, t, mode); rerr == nil {
			err = fn(cctx, db, admin, conn)
		}
	}
	return apperr.FromEngine(err, "instance")
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
	var tables []engine.TableInfo
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) (err error) {
			tables, err = admin.ListTables(ctx, conn, db.Name)
			return err
		})
	return tables, err
}

// TableRows returns one page of table data.
func (s *Service) TableRows(ctx context.Context, databaseID, table string, limit, offset int) (*engine.RowsPage, error) {
	var page *engine.RowsPage
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) (err error) {
			page, err = admin.TableRows(ctx, conn, db.Name, table, limit, offset)
			return err
		})
	return page, err
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
	var schema *engine.TableSchema
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) (err error) {
			schema, err = admin.TableSchema(ctx, conn, db.Name, table)
			return err
		})
	return schema, err
}

// QueryInput is one console run: one or more statements.
type QueryInput struct {
	DatabaseID string
	SQL        string
	Limit      int
	// AllowWrite comes from the caller's database:write permission on this
	// database; without it every statement must be a read.
	AllowWrite bool
	// QueryID, when set, registers the run so its owner can cancel it.
	QueryID string
	UserID  uuid.UUID
}

// BatchFailure says which statement of a run failed (1-based) and why.
type BatchFailure struct {
	Statement int    `json:"statement"`
	Message   string `json:"message"`
}

// QueryOutput holds the results of the statements that ran. When a later
// statement fails, the earlier results are kept and Error describes the
// failure (a failing first statement is returned as an error instead).
type QueryOutput struct {
	Results []engine.QueryResult `json:"results"`
	Error   *BatchFailure        `json:"error,omitempty"`
}

// Query runs a console script. Reads run as the read-only role; a script with
// any write runs entirely as the read-write role, on one session, stopping at
// the first failing statement.
func (s *Service) Query(ctx context.Context, in QueryInput) (*QueryOutput, error) {
	stmts := engine.SplitStatements(in.SQL)
	if len(stmts) == 0 {
		return nil, apperr.Invalid("sql", "query is empty")
	}
	mode := dbaccessdom.ModeRead
	for i, st := range stmts {
		if !engine.IsReadOnly(st) {
			if !in.AllowWrite {
				return nil, apperr.Forbidden(fmt.Sprintf("statement %d looks like a write statement; running it requires database:write", i+1))
			}
			mode = dbaccessdom.ModeWrite
		}
	}

	ctx, done := s.track(ctx, in)
	defer done()

	start := time.Now()
	out := &QueryOutput{Results: []engine.QueryResult{}}
	var batchErr error
	err := s.scoped(ctx, in.DatabaseID, mode, queryTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			res, err := admin.QueryBatch(ctx, conn, db.Name, stmts, in.Limit, mode == dbaccessdom.ModeWrite,
				func(pid int64) { s.setSessionID(in.QueryID, pid) })
			out.Results = res
			var be *engine.BatchError
			if errors.As(err, &be) && be.Index > 0 {
				// Earlier statements already ran: never retry (that could
				// repeat writes); report the failure alongside the results.
				batchErr = be
				return nil
			}
			return err
		})
	s.recordHistory(in, len(stmts), out, time.Since(start), firstErr(err, batchErr))
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, apperr.Conflict("the query was cancelled")
		}
		return nil, err
	}
	if batchErr != nil {
		var be *engine.BatchError
		errors.As(batchErr, &be)
		out.Error = &BatchFailure{Statement: be.Index + 1, Message: apperr.EngineMessage(be.Err)}
	}
	return out, nil
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// ---- Console: cancellation and history ----

type runningQuery struct {
	user       uuid.UUID
	databaseID string
	cancel     context.CancelFunc
	sessionID  int64
}

// track registers a cancellable run under its query id.
func (s *Service) track(ctx context.Context, in QueryInput) (context.Context, func()) {
	if in.QueryID == "" {
		return ctx, func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	s.runningMu.Lock()
	if s.running == nil {
		s.running = map[string]*runningQuery{}
	}
	s.running[in.QueryID] = &runningQuery{user: in.UserID, databaseID: in.DatabaseID, cancel: cancel}
	s.runningMu.Unlock()
	return ctx, func() {
		s.runningMu.Lock()
		delete(s.running, in.QueryID)
		s.runningMu.Unlock()
		cancel()
	}
}

func (s *Service) setSessionID(queryID string, id int64) {
	if queryID == "" {
		return
	}
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	if q, ok := s.running[queryID]; ok {
		q.sessionID = id
	}
}

// CancelQuery stops a running console query of the caller's. The request is
// cancelled locally, and the statement is killed on the server too (MySQL
// would otherwise keep executing it after the client disconnects).
//
// The registry is in memory: with several API replicas, cancel reaches the
// query only when it lands on the replica running it.
func (s *Service) CancelQuery(ctx context.Context, databaseID, queryID string, userID uuid.UUID) error {
	s.runningMu.Lock()
	q, ok := s.running[queryID]
	s.runningMu.Unlock()
	if !ok || q.user != userID || q.databaseID != databaseID {
		return apperr.NotFound("no running query with that id (it may have finished)")
	}
	if q.sessionID > 0 {
		if did, err := uuid.Parse(databaseID); err == nil {
			if db, err := s.databases.GetByID(ctx, did); err == nil {
				if mon, conn, err := s.monitor(ctx, db.InstanceID.String()); err == nil {
					kctx, cancel := context.WithTimeout(ctx, 5*time.Second)
					_ = mon.KillProcess(kctx, conn, q.sessionID, false)
					cancel()
				}
			}
		}
	}
	q.cancel()
	return nil
}

// History stores console runs; optional.
type History interface {
	AddHistory(ctx context.Context, e *consoledom.HistoryEntry) error
}

// SetHistory enables console history recording.
func (s *Service) SetHistory(h History) { s.history = h }

func (s *Service) recordHistory(in QueryInput, statements int, out *QueryOutput, d time.Duration, err error) {
	if s.history == nil || in.UserID == uuid.Nil {
		return
	}
	did, perr := uuid.Parse(in.DatabaseID)
	if perr != nil {
		return
	}
	e := &consoledom.HistoryEntry{UserID: in.UserID, DatabaseID: did, SQL: in.SQL, Statements: statements, DurationMS: d.Milliseconds()}
	for _, r := range out.Results {
		e.RowCount += int64(r.RowCount) + r.RowsAffected
	}
	if err != nil {
		msg := apperr.EngineMessage(err)
		e.Error = &msg
	}
	// History must never fail or slow the query it describes.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if herr := s.history.AddHistory(ctx, e); herr != nil {
		slog.Warn("record query history", "error", herr.Error())
	}
}

// ExportTableCSV streams a whole table to w as CSV. onStart is invoked once the
// result set opens successfully, before any bytes are written.
func (s *Service) ExportTableCSV(ctx context.Context, databaseID, table string, w io.Writer, onStart func()) (int64, error) {
	var n int64
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, exportTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) (err error) {
			n, err = admin.ExportCSV(ctx, conn, db.Name, table, "", w, onceStart(onStart))
			return err
		})
	return n, err
}

// ExportQueryCSV streams a read-only query's result to w as CSV.
func (s *Service) ExportQueryCSV(ctx context.Context, databaseID, sql string, w io.Writer, onStart func()) (int64, error) {
	var n int64
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, exportTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) (err error) {
			n, err = admin.ExportCSV(ctx, conn, db.Name, "", sql, w, onceStart(onStart))
			return err
		})
	return n, err
}

// onceStart guards the export start callback: a permission-denied retry must
// not write response headers twice.
func onceStart(f func()) func() {
	if f == nil {
		return nil
	}
	var once sync.Once
	return func() { once.Do(f) }
}

// DropAccessRoles removes a dropped database's console roles from its
// instance. Best effort: failures are logged, never surfaced, because the
// database is already gone and a role without grants is inert.
func (s *Service) DropAccessRoles(ctx context.Context, databaseID, instanceID uuid.UUID) {
	dropper, ok := s.access.(interface {
		Drop(ctx context.Context, t dbaccessapp.Target) error
	})
	if !ok {
		return
	}
	t := dbaccessapp.Target{Database: &databasedom.Database{ID: databaseID}}
	if inst, admin, root, err := s.target(ctx, instanceID.String()); err == nil {
		t.Instance, t.Admin, t.Root = inst, admin, root
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	if err := dropper.Drop(cctx, t); err != nil {
		slog.Warn("drop database access roles", "database_id", databaseID, "error", err.Error())
	}
}

// ---- Instance-level: monitoring ----

// monitor resolves the instance's monitoring surface and admin connection.
func (s *Service) monitor(ctx context.Context, instanceID string) (engine.Monitor, engine.ConnParams, error) {
	inst, _, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return nil, engine.ConnParams{}, err
	}
	mon, err := engine.MonitorFor(string(inst.Engine))
	if err != nil {
		return nil, engine.ConnParams{}, apperr.Invalid("engine", err.Error())
	}
	return mon, conn, nil
}

// Processes lists the instance's sessions.
func (s *Service) Processes(ctx context.Context, instanceID string) ([]engine.Process, error) {
	mon, conn, err := s.monitor(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	out, err := mon.Processes(cctx, conn)
	return out, apperr.FromEngine(err, "instance")
}

// KillProcess cancels a session's statement, or terminates the session.
func (s *Service) KillProcess(ctx context.Context, instanceID string, pid int64, connection bool) error {
	mon, conn, err := s.monitor(ctx, instanceID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return apperr.FromEngine(mon.KillProcess(cctx, conn, pid, connection), "pid")
}

// ServerStatus returns the instance's status counters.
func (s *Service) ServerStatus(ctx context.Context, instanceID string) ([]engine.Setting, error) {
	mon, conn, err := s.monitor(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	out, err := mon.ServerStatus(cctx, conn)
	return out, apperr.FromEngine(err, "instance")
}

// Variables returns the instance's configuration settings.
func (s *Service) Variables(ctx context.Context, instanceID string) ([]engine.Setting, error) {
	mon, conn, err := s.monitor(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	out, err := mon.Variables(cctx, conn)
	return out, apperr.FromEngine(err, "instance")
}

// ---- Database-level: data editing ----

// importTimeout bounds a CSV import (parsing and inserting in one transaction).
const importTimeout = 10 * time.Minute

func dataEditor(admin engine.Admin) (engine.DataEditor, error) {
	ed, ok := admin.(engine.DataEditor)
	if !ok {
		return nil, apperr.Invalid("engine", "this engine does not support data editing")
	}
	return ed, nil
}

// BrowseRows returns a filtered, sorted page of a table, plus the row key the
// dashboard needs for editing.
func (s *Service) BrowseRows(ctx context.Context, databaseID string, req engine.BrowseRequest) (*engine.BrowseResult, error) {
	var res *engine.BrowseResult
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			ed, err := dataEditor(admin)
			if err != nil {
				return err
			}
			res, err = ed.BrowseRows(ctx, conn, db.Name, req)
			return err
		})
	return res, err
}

// InsertRow inserts one row.
func (s *Service) InsertRow(ctx context.Context, databaseID, table string, values engine.RowValues) error {
	return s.scoped(ctx, databaseID, dbaccessdom.ModeWrite, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			ed, err := dataEditor(admin)
			if err != nil {
				return err
			}
			return ed.InsertRow(ctx, conn, db.Name, table, values)
		})
}

// UpdateRow updates the row identified by key.
func (s *Service) UpdateRow(ctx context.Context, databaseID, table string, key, values engine.RowValues) error {
	return s.scoped(ctx, databaseID, dbaccessdom.ModeWrite, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			ed, err := dataEditor(admin)
			if err != nil {
				return err
			}
			return ed.UpdateRow(ctx, conn, db.Name, table, key, values)
		})
}

// DeleteRow deletes the row identified by key.
func (s *Service) DeleteRow(ctx context.Context, databaseID, table string, key engine.RowValues) error {
	return s.scoped(ctx, databaseID, dbaccessdom.ModeWrite, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			ed, err := dataEditor(admin)
			if err != nil {
				return err
			}
			return ed.DeleteRow(ctx, conn, db.Name, table, key)
		})
}

// ImportCSV loads a CSV into a table, all or nothing. The reader is consumed
// once, so a permission-denied retry is not attempted after reading began.
func (s *Service) ImportCSV(ctx context.Context, databaseID, table string, r io.Reader, opts engine.ImportOptions) (int64, error) {
	var n int64
	consumed := false
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeWrite, importTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) (err error) {
			if consumed {
				return apperr.Conflict("the import was refused by the database; retry the upload")
			}
			consumed = true
			ed, err := dataEditor(admin)
			if err != nil {
				return err
			}
			n, err = ed.ImportCSV(ctx, conn, db.Name, table, r, opts)
			return err
		})
	return n, err
}

// ---- Database-level: structure ----

// ddlTimeout bounds a DDL statement (an ALTER on a large table rewrites it).
const ddlTimeout = 5 * time.Minute

func structure(admin engine.Admin) (engine.Structure, error) {
	st, ok := admin.(engine.Structure)
	if !ok {
		return nil, apperr.Invalid("engine", "this engine does not support structure editing")
	}
	return st, nil
}

// withStructure runs a DDL change as the database's read-write role.
func (s *Service) withStructure(ctx context.Context, databaseID string,
	fn func(ctx context.Context, st engine.Structure, conn engine.ConnParams, db string) error) error {
	return s.scoped(ctx, databaseID, dbaccessdom.ModeWrite, ddlTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			st, err := structure(admin)
			if err != nil {
				return err
			}
			if err := fn(ctx, st, conn, db.Name); err != nil {
				return err
			}
			s.refreshReadGrants(ctx, db)
			return nil
		})
}

// refreshReadGrants re-applies the read-only role's grants after DDL, so a
// table just created through Fleetdock is immediately browsable. (On
// PostgreSQL a grant covers only tables that existed when it was made.)
// Best effort: browsing also repairs grants on its own when refused.
func (s *Service) refreshReadGrants(ctx context.Context, db *databasedom.Database) {
	inst, admin, root, err := s.target(ctx, db.InstanceID.String())
	if err != nil || inst.Engine != instancedom.EnginePostgres {
		return
	}
	t := dbaccessapp.Target{Instance: inst, Database: db, Admin: admin, Root: root}
	if err := s.access.Reapply(ctx, t, dbaccessdom.ModeRead); err != nil {
		slog.Warn("refresh read grants after DDL", "database_id", db.ID, "error", err.Error())
	}
}

// CreateTable creates a table from a spec.
func (s *Service) CreateTable(ctx context.Context, databaseID string, spec engine.TableSpec) error {
	return s.withStructure(ctx, databaseID, func(ctx context.Context, st engine.Structure, c engine.ConnParams, db string) error {
		return st.CreateTable(ctx, c, db, spec)
	})
}

// AlterTable applies column / foreign-key changes.
func (s *Service) AlterTable(ctx context.Context, databaseID, table string, ops []engine.AlterOp) error {
	return s.withStructure(ctx, databaseID, func(ctx context.Context, st engine.Structure, c engine.ConnParams, db string) error {
		return st.AlterTable(ctx, c, db, table, ops)
	})
}

// DropTable drops a table.
func (s *Service) DropTable(ctx context.Context, databaseID, table string) error {
	return s.withStructure(ctx, databaseID, func(ctx context.Context, st engine.Structure, c engine.ConnParams, db string) error {
		return st.DropTable(ctx, c, db, table)
	})
}

// TruncateTable removes every row of a table.
func (s *Service) TruncateTable(ctx context.Context, databaseID, table string) error {
	return s.withStructure(ctx, databaseID, func(ctx context.Context, st engine.Structure, c engine.ConnParams, db string) error {
		return st.TruncateTable(ctx, c, db, table)
	})
}

// RenameTable renames a table.
func (s *Service) RenameTable(ctx context.Context, databaseID, table, newName string) error {
	return s.withStructure(ctx, databaseID, func(ctx context.Context, st engine.Structure, c engine.ConnParams, db string) error {
		return st.RenameTable(ctx, c, db, table, newName)
	})
}

// CreateIndex creates an index.
func (s *Service) CreateIndex(ctx context.Context, databaseID, table string, spec engine.IndexSpec) error {
	return s.withStructure(ctx, databaseID, func(ctx context.Context, st engine.Structure, c engine.ConnParams, db string) error {
		return st.CreateIndex(ctx, c, db, table, spec)
	})
}

// DropIndex drops an index.
func (s *Service) DropIndex(ctx context.Context, databaseID, table, index string) error {
	return s.withStructure(ctx, databaseID, func(ctx context.Context, st engine.Structure, c engine.ConnParams, db string) error {
		return st.DropIndex(ctx, c, db, table, index)
	})
}

// ForeignKeys lists a table's foreign keys.
func (s *Service) ForeignKeys(ctx context.Context, databaseID, table string) ([]engine.ForeignKey, error) {
	var out []engine.ForeignKey
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			st, err := structure(admin)
			if err != nil {
				return err
			}
			out, err = st.ForeignKeys(ctx, conn, db.Name, table)
			return err
		})
	return out, err
}

// Objects lists views, routines, triggers, sequences and events.
func (s *Service) Objects(ctx context.Context, databaseID string) ([]engine.DBObject, error) {
	var out []engine.DBObject
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			st, err := structure(admin)
			if err != nil {
				return err
			}
			out, err = st.Objects(ctx, conn, db.Name)
			return err
		})
	return out, err
}

// WriteRoleCredentials returns (provisioning if needed) the database's
// read-write console role login, for operations that execute user-supplied
// SQL — such as imports — without instance-admin rights.
func (s *Service) WriteRoleCredentials(ctx context.Context, databaseID uuid.UUID) (string, string, error) {
	db, err := s.databases.GetByID(ctx, databaseID)
	if err != nil {
		return "", "", err
	}
	inst, admin, root, err := s.target(ctx, db.InstanceID.String())
	if err != nil {
		return "", "", err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	conn, err := s.access.Conn(cctx, dbaccessapp.Target{Instance: inst, Database: db, Admin: admin, Root: root}, dbaccessdom.ModeWrite)
	if err != nil {
		return "", "", err
	}
	return conn.User, conn.Password, nil
}

// SetDBUserPassword changes a database account's password. The instance's
// own admin login and Fleetdock's console roles are refused: changing them
// here would break the control plane's access (edit the instance instead).
func (s *Service) SetDBUserPassword(ctx context.Context, instanceID, user, host, password string) error {
	inst, admin, conn, err := s.target(ctx, instanceID)
	if err != nil {
		return err
	}
	if password == "" {
		return apperr.Invalid("password", "password is required")
	}
	if inst.Username != nil && user == *inst.Username {
		return apperr.Invalid("username", "this is the instance's admin login; change it from the instance settings so Fleetdock keeps working")
	}
	if strings.HasPrefix(user, "fleetdock_ro_") || strings.HasPrefix(user, "fleetdock_rw_") {
		return apperr.Invalid("username", "this account is managed by Fleetdock")
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return apperr.FromEngine(engine.RotatePassword(cctx, admin, conn, user, host, password), "username")
}
