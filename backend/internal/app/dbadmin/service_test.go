package dbadminapp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	dbaccessapp "github.com/Fleetdock/fleetdock/backend/internal/app/dbaccess"
	consoledom "github.com/Fleetdock/fleetdock/backend/internal/domain/console"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	dbaccessdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbaccess"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

type oneInstance struct {
	instancedom.Repository
	inst *instancedom.Instance
}

func (r oneInstance) GetByID(context.Context, uuid.UUID) (*instancedom.Instance, error) {
	c := *r.inst
	return &c, nil
}

type oneDatabase struct {
	databasedom.Repository
	db *databasedom.Database
}

func (r oneDatabase) GetByID(context.Context, uuid.UUID) (*databasedom.Database, error) {
	c := *r.db
	return &c, nil
}

type staticSecrets struct{}

func (staticSecrets) Get(context.Context, string) ([]byte, error) { return []byte("rootpw"), nil }

// fakeAccess hands out a recognisable role per mode and counts re-grants.
type fakeAccess struct {
	modes    []dbaccessdom.Mode
	reapply  int
	lastRoot engine.ConnParams
}

func (a *fakeAccess) Conn(_ context.Context, t dbaccessapp.Target, m dbaccessdom.Mode) (engine.ConnParams, error) {
	a.modes = append(a.modes, m)
	a.lastRoot = t.Root
	c := t.Root
	c.User, c.Password = "fleetdock_"+string(m), "rolepw"
	return c, nil
}

func (a *fakeAccess) Reapply(context.Context, dbaccessapp.Target, dbaccessdom.Mode) error {
	a.reapply++
	return nil
}

// fakeEngine records which user ran each batch.
type fakeEngine struct {
	engine.Admin
	users      []string
	denyFirst  bool
	writeFlags []bool
	// failAt makes statement failAt (0-based) of every batch fail with a
	// permission error; -1 disables.
	failAt  int
	batches [][]string
	block   chan struct{}
}

func (e *fakeEngine) QueryBatch(ctx context.Context, p engine.ConnParams, _ string, stmts []string, _ int, allowWrite bool, onSession func(int64)) ([]engine.QueryResult, error) {
	e.users = append(e.users, p.User)
	e.writeFlags = append(e.writeFlags, allowWrite)
	e.batches = append(e.batches, stmts)
	if onSession != nil {
		onSession(42)
	}
	if e.block != nil {
		select {
		case <-e.block:
		case <-ctx.Done():
			return nil, &engine.BatchError{Index: 0, Err: ctx.Err()}
		}
	}
	denied := &mysql.MySQLError{Number: 1142, Message: "command denied"}
	if e.denyFirst {
		e.denyFirst = false
		return nil, &engine.BatchError{Index: 0, Err: denied}
	}
	var out []engine.QueryResult
	for i := range stmts {
		if i == e.failAt {
			return out, &engine.BatchError{Index: i, Err: denied}
		}
		out = append(out, engine.QueryResult{RowsAffected: 1})
	}
	return out, nil
}

func fixture(t *testing.T, status databasedom.Status) (*Service, *fakeAccess, *fakeEngine) {
	t.Helper()
	user, host := "root", "db.example.com"
	ref := "instance/x/root"
	inst := &instancedom.Instance{ID: uuid.New(), Engine: "fake-test-engine", Kind: instancedom.KindExternal,
		Host: &host, Port: 3306, Username: &user, RootSecretRef: &ref}
	db := &databasedom.Database{ID: uuid.New(), InstanceID: inst.ID, Name: "app", Status: status}
	eng := &fakeEngine{failAt: -1}
	engine.Register("fake-test-engine", fakeClient{eng})
	acc := &fakeAccess{}
	return NewService(oneInstance{inst: inst}, oneDatabase{db: db}, nil, staticSecrets{}, acc), acc, eng
}

// fakeClient satisfies engine.Client by delegating admin calls to fakeEngine.
type fakeClient struct{ *fakeEngine }

func (fakeClient) Ping(context.Context, engine.ConnParams) (string, error) { return "", nil }
func (fakeClient) ListDatabases(context.Context, engine.ConnParams) ([]engine.DatabaseInfo, error) {
	return nil, nil
}
func (fakeClient) CreateDatabase(context.Context, engine.ConnParams, string, string, string) error {
	return nil
}
func (fakeClient) DropDatabase(context.Context, engine.ConnParams, string) error { return nil }
func (fakeClient) CountTables(context.Context, engine.ConnParams, string) (int, error) {
	return 0, nil
}
func (fakeClient) DumpArgs(engine.ConnParams, string) ([]string, []string, []string) {
	return nil, nil, nil
}
func (fakeClient) RestoreArgs(engine.ConnParams, string) ([]string, []string, []string) {
	return nil, nil, nil
}

func TestQuery_ReadRunsAsReadOnlyRole(t *testing.T) {
	svc, acc, eng := fixture(t, databasedom.StatusActive)
	if _, err := svc.Query(context.Background(), QueryInput{DatabaseID: uuid.NewString(), SQL: "SELECT 1", Limit: 10, AllowWrite: true}); err != nil {
		t.Fatal(err)
	}
	if len(acc.modes) != 1 || acc.modes[0] != dbaccessdom.ModeRead {
		t.Fatalf("modes = %v, want [ro] even when the caller may write", acc.modes)
	}
	if eng.users[0] != "fleetdock_ro" || eng.writeFlags[0] {
		t.Errorf("ran as %q (write=%v), want fleetdock_ro read-only", eng.users[0], eng.writeFlags[0])
	}
}

func TestQuery_WriteRunsAsReadWriteRole(t *testing.T) {
	svc, acc, eng := fixture(t, databasedom.StatusActive)
	if _, err := svc.Query(context.Background(), QueryInput{DatabaseID: uuid.NewString(), SQL: "DELETE FROM t", Limit: 10, AllowWrite: true}); err != nil {
		t.Fatal(err)
	}
	if acc.modes[0] != dbaccessdom.ModeWrite || eng.users[0] != "fleetdock_rw" {
		t.Errorf("mode %v user %q, want rw", acc.modes, eng.users)
	}
}

func TestQuery_WriteWithoutPermissionIsForbidden(t *testing.T) {
	svc, acc, _ := fixture(t, databasedom.StatusActive)
	_, err := svc.Query(context.Background(), QueryInput{DatabaseID: uuid.NewString(), SQL: "DROP TABLE t", Limit: 10, AllowWrite: false})
	if apperr.KindOf(err) != apperr.KindForbidden {
		t.Fatalf("err = %v, want forbidden", err)
	}
	if len(acc.modes) != 0 {
		t.Error("no role should be resolved for a refused write")
	}
}

func TestQuery_LockedDatabaseRefusesWrites(t *testing.T) {
	svc, _, _ := fixture(t, databasedom.StatusLocked)
	_, err := svc.Query(context.Background(), QueryInput{DatabaseID: uuid.NewString(), SQL: "UPDATE t SET a = 1", Limit: 10, AllowWrite: true})
	if apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("err = %v, want conflict for a locked database", err)
	}
	if _, err := svc.Query(context.Background(), QueryInput{DatabaseID: uuid.NewString(), SQL: "SELECT 1", Limit: 10, AllowWrite: true}); err != nil {
		t.Fatalf("reads on a locked database should still work: %v", err)
	}
}

func TestQuery_PermissionDeniedReappliesGrantsAndRetries(t *testing.T) {
	svc, acc, eng := fixture(t, databasedom.StatusActive)
	eng.denyFirst = true
	if _, err := svc.Query(context.Background(), QueryInput{DatabaseID: uuid.NewString(), SQL: "SELECT * FROM new_table", Limit: 10, AllowWrite: false}); err != nil {
		t.Fatalf("expected the retry to succeed: %v", err)
	}
	if acc.reapply != 1 || len(eng.users) != 2 {
		t.Errorf("reapply=%d calls=%d, want 1 re-grant and 2 attempts", acc.reapply, len(eng.users))
	}
}

func TestQuery_ScriptWithAWriteRunsAsWriteRole(t *testing.T) {
	svc, acc, eng := fixture(t, databasedom.StatusActive)
	out, err := svc.Query(context.Background(), QueryInput{DatabaseID: uuid.NewString(), SQL: "SELECT 1; UPDATE t SET a = 1; SELECT 2", AllowWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	if acc.modes[0] != dbaccessdom.ModeWrite || len(eng.batches[0]) != 3 || len(out.Results) != 3 {
		t.Errorf("mode=%v batch=%v results=%d", acc.modes, eng.batches, len(out.Results))
	}
}

func TestQuery_WriteInScriptWithoutPermissionNamesTheStatement(t *testing.T) {
	svc, _, eng := fixture(t, databasedom.StatusActive)
	_, err := svc.Query(context.Background(), QueryInput{DatabaseID: uuid.NewString(), SQL: "SELECT 1; DELETE FROM t", AllowWrite: false})
	if apperr.KindOf(err) != apperr.KindForbidden || !strings.Contains(err.Error(), "statement 2") {
		t.Fatalf("err = %v", err)
	}
	if len(eng.batches) != 0 {
		t.Error("nothing may run when any statement is refused")
	}
}

func TestQuery_LaterFailureKeepsResultsAndNeverRetries(t *testing.T) {
	svc, acc, eng := fixture(t, databasedom.StatusActive)
	eng.failAt = 1
	out, err := svc.Query(context.Background(), QueryInput{DatabaseID: uuid.NewString(), SQL: "UPDATE t SET a = 1; UPDATE u SET b = 2", AllowWrite: true})
	if err != nil {
		t.Fatalf("a later failure is reported in the output, not as an error: %v", err)
	}
	if len(out.Results) != 1 || out.Error == nil || out.Error.Statement != 2 {
		t.Errorf("out = %+v", out)
	}
	if acc.reapply != 0 || len(eng.batches) != 1 {
		t.Errorf("reapply=%d runs=%d: statement 1 already ran, so the batch must not be retried", acc.reapply, len(eng.batches))
	}
}

type memHistory struct{ entries []*consoledom.HistoryEntry }

func (h *memHistory) AddHistory(_ context.Context, e *consoledom.HistoryEntry) error {
	h.entries = append(h.entries, e)
	return nil
}

func TestQuery_RecordsHistory(t *testing.T) {
	svc, _, eng := fixture(t, databasedom.StatusActive)
	h := &memHistory{}
	svc.SetHistory(h)
	eng.failAt = 0
	user := uuid.New()
	_, _ = svc.Query(context.Background(), QueryInput{DatabaseID: uuid.NewString(), SQL: "SELECT 1", UserID: user})
	if len(h.entries) != 1 || h.entries[0].UserID != user || h.entries[0].Error == nil {
		t.Fatalf("history = %+v", h.entries)
	}
}

func TestCancelQuery_OnlyTheOwner(t *testing.T) {
	svc, _, eng := fixture(t, databasedom.StatusActive)
	eng.block = make(chan struct{})
	owner, dbID, qid := uuid.New(), uuid.NewString(), uuid.NewString()

	done := make(chan error, 1)
	go func() {
		_, err := svc.Query(context.Background(), QueryInput{DatabaseID: dbID, SQL: "SELECT SLEEP(100)", QueryID: qid, UserID: owner})
		done <- err
	}()
	// Wait until the run is registered.
	for i := 0; i < 100; i++ {
		svc.runningMu.Lock()
		_, ok := svc.running[qid]
		svc.runningMu.Unlock()
		if ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := svc.CancelQuery(context.Background(), dbID, qid, uuid.New()); apperr.KindOf(err) != apperr.KindNotFound {
		t.Errorf("another user's cancel = %v, want not found", err)
	}
	if err := svc.CancelQuery(context.Background(), dbID, qid, owner); err != nil {
		t.Fatalf("owner cancel: %v", err)
	}
	select {
	case err := <-done:
		if apperr.KindOf(err) != apperr.KindConflict {
			t.Errorf("cancelled query err = %v, want conflict (cancelled)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("query was not cancelled")
	}
}
