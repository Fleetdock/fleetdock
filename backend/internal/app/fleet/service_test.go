package fleetapp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	authapp "github.com/Fleetdock/fleetdock/backend/internal/app/auth"
	dbadminapp "github.com/Fleetdock/fleetdock/backend/internal/app/dbadmin"
	"github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	fleetdom "github.com/Fleetdock/fleetdock/backend/internal/domain/fleet"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

type fakeRepo struct {
	fleetdom.Repository
	runs    map[uuid.UUID]*fleetdom.Run
	targets map[uuid.UUID][]fleetdom.Target
}

func (f *fakeRepo) Create(_ context.Context, r *fleetdom.Run, ts []fleetdom.Target) error {
	r.ID, r.Status, r.Total = uuid.New(), fleetdom.RunRunning, len(ts)
	f.runs[r.ID], f.targets[r.ID] = r, ts
	return nil
}

func (f *fakeRepo) Get(_ context.Context, id uuid.UUID) (*fleetdom.Run, error) {
	if r, ok := f.runs[id]; ok {
		return r, nil
	}
	return nil, apperr.NotFound("fleet run not found")
}

func (f *fakeRepo) Targets(_ context.Context, id uuid.UUID, _ bool) ([]fleetdom.Target, error) {
	return f.targets[id], nil
}

type world struct {
	dbs      map[uuid.UUID]*databasedom.Database
	inst     *instancedom.Instance
	info     dbadminapp.DataAccessInfo
	users    map[uuid.UUID]*authapp.Principal
	queried  []dbadminapp.QueryInput
	queryOut *dbadminapp.QueryOutput
}

func (w *world) GetByID(_ context.Context, id uuid.UUID) (*databasedom.Database, error) {
	if d, ok := w.dbs[id]; ok {
		return d, nil
	}
	return nil, apperr.NotFound("database not found")
}

type instances struct{ w *world }

func (i instances) GetByID(context.Context, uuid.UUID) (*instancedom.Instance, error) {
	return i.w.inst, nil
}

func (w *world) Ancestry(_ context.Context, t authz.ResourceType, id uuid.UUID) (authz.Ancestry, error) {
	if t == authz.ResourceDatabase {
		return authz.Ancestry{Covers: []authz.Scope{{Type: authz.ScopeDatabase, ID: id}}}, nil
	}
	return authz.Ancestry{}, nil
}

func (w *world) DataAccess(context.Context, string) (dbadminapp.DataAccessInfo, error) {
	return w.info, nil
}

func (w *world) PrincipalForUser(_ context.Context, id uuid.UUID, _ []string) (*authapp.Principal, error) {
	if p, ok := w.users[id]; ok {
		return p, nil
	}
	return nil, apperr.Unauthorized("account is suspended")
}

func (w *world) Query(_ context.Context, in dbadminapp.QueryInput) (*dbadminapp.QueryOutput, error) {
	w.queried = append(w.queried, in)
	return w.queryOut, nil
}

func (w *world) CancelQuery(context.Context, string, string, uuid.UUID) error { return nil }

func grantOn(user uuid.UUID, perms map[uuid.UUID][]string) *authapp.Principal {
	var gs []authz.Grant
	for db, ps := range perms {
		for _, p := range ps {
			gs = append(gs, authz.Grant{Permission: p, Scope: authz.Scope{Type: authz.ScopeDatabase, ID: db}})
		}
	}
	return authapp.NewPrincipalWithGrants(user, "u@example.com", gs)
}

func setup(t *testing.T) (*Service, *world, *fakeRepo, uuid.UUID, uuid.UUID) {
	t.Helper()
	a, b := uuid.New(), uuid.New()
	inst := &instancedom.Instance{ID: uuid.New(), Name: "pg-1", Engine: instancedom.EnginePostgres}
	w := &world{
		dbs: map[uuid.UUID]*databasedom.Database{
			a: {ID: a, Name: "shop_a", InstanceID: inst.ID}, b: {ID: b, Name: "shop_b", InstanceID: inst.ID},
		},
		inst:  inst,
		info:  dbadminapp.DataAccessInfo{InstanceID: inst.ID, Mode: instancedom.DataAccessManaged},
		users: map[uuid.UUID]*authapp.Principal{},
	}
	repo := &fakeRepo{runs: map[uuid.UUID]*fleetdom.Run{}, targets: map[uuid.UUID][]fleetdom.Target{}}
	svc := NewService(Deps{Repo: repo, Databases: w, Instances: instances{w}, Ancestry: w, DataAccess: w, Principals: w, Queries: w})
	return svc, w, repo, a, b
}

func TestCreateAuthorizesEveryDatabase(t *testing.T) {
	ctx := context.Background()
	svc, _, repo, a, b := setup(t)
	user := uuid.New()
	readBoth := grantOn(user, map[uuid.UUID][]string{a: {"database:read"}, b: {"database:read"}})

	run, err := svc.Create(ctx, readBoth, CreateInput{SQL: "SELECT 1", DatabaseIDs: []string{a.String(), b.String(), a.String()}})
	if err != nil {
		t.Fatal(err)
	}
	if run.Writes || run.Canary || run.StopOnError || run.Concurrency != 4 || run.TimeoutSeconds != 60 || run.RowLimit != 100 {
		t.Fatalf("read defaults = %+v", run)
	}
	if ts := repo.targets[run.ID]; len(ts) != 2 || ts[0].DatabaseName != "shop_a" || ts[1].InstanceName != "pg-1" {
		t.Fatalf("targets = %+v", ts)
	}

	// Writes need database:write on every database.
	_, err = svc.Create(ctx, readBoth, CreateInput{SQL: "UPDATE t SET x = 1", DatabaseIDs: []string{a.String()}})
	if apperr.KindOf(err) != apperr.KindForbidden || !strings.Contains(err.Error(), "shop_a") {
		t.Fatalf("write without database:write: %v", err)
	}
	writer := grantOn(user, map[uuid.UUID][]string{a: {"database:read", "database:write"}, b: {"database:read", "database:write"}})
	run, err = svc.Create(ctx, writer, CreateInput{SQL: "SELECT 1; UPDATE t SET x = 1", DatabaseIDs: []string{a.String(), b.String()}, Concurrency: 99})
	if err != nil {
		t.Fatal(err)
	}
	if !run.Writes || !run.Canary || !run.StopOnError || run.Concurrency != fleetdom.MaxConcurrency {
		t.Fatalf("write defaults = %+v", run)
	}

	// A database the user cannot see refuses the whole run.
	onlyA := grantOn(user, map[uuid.UUID][]string{a: {"database:read"}})
	if _, err := svc.Create(ctx, onlyA, CreateInput{SQL: "SELECT 1", DatabaseIDs: []string{a.String(), b.String()}}); apperr.KindOf(err) != apperr.KindForbidden {
		t.Fatalf("unauthorized database: %v", err)
	}
	if _, err := svc.Create(ctx, onlyA, CreateInput{SQL: " ;", DatabaseIDs: []string{a.String()}}); apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("empty script: %v", err)
	}
}

func TestCreateHonoursDataAccessMode(t *testing.T) {
	svc, w, _, a, _ := setup(t)
	w.info.Mode = instancedom.DataAccessAdmin
	p := grantOn(uuid.New(), map[uuid.UUID][]string{a: {"database:read"}})
	if _, err := svc.Create(context.Background(), p, CreateInput{SQL: "SELECT 1", DatabaseIDs: []string{a.String()}}); apperr.KindOf(err) != apperr.KindForbidden {
		t.Fatalf("admin-login instance without instance:write: %v", err)
	}
}

func TestRunTargetRechecksAccess(t *testing.T) {
	ctx := context.Background()
	svc, w, _, a, _ := setup(t)
	user := uuid.New()
	w.users[user] = grantOn(user, map[uuid.UUID][]string{a: {"database:read"}})
	w.queryOut = &dbadminapp.QueryOutput{Results: []engine.QueryResult{{Columns: []string{"n"}, Rows: [][]*string{{sp("1")}}}}}
	claim := fleetdom.Claim{
		Run:    fleetdom.Run{CreatedBy: user, SQL: "SELECT 1", RowLimit: 10, TimeoutSeconds: 5, Total: 1},
		Target: fleetdom.Target{DatabaseID: &a},
	}
	o := svc.runTarget(ctx, claim, func(string, string) {})
	if o.Status != fleetdom.TargetSucceeded || len(w.queried) != 1 || w.queried[0].Timeout != 5*time.Second || w.queried[0].AllowWrite {
		t.Fatalf("outcome = %+v, queried %+v", o, w.queried)
	}

	// The creator lost write access since the run was created.
	claim.Run.Writes = true
	if o := svc.runTarget(ctx, claim, func(string, string) {}); o.Status != fleetdom.TargetFailed || !strings.Contains(o.Error, "not run") {
		t.Fatalf("write without access: %+v", o)
	}
	// The creator was suspended.
	delete(w.users, user)
	if o := svc.runTarget(ctx, claim, func(string, string) {}); o.Status != fleetdom.TargetFailed || len(w.queried) != 1 {
		t.Fatalf("suspended user: %+v", o)
	}
}

func sp(s string) *string { return &s }

func TestEncodeResultsStripsNULAndTrims(t *testing.T) {
	rows := make([][]*string, 200)
	for i := range rows {
		rows[i] = []*string{sp(strings.Repeat("x", 100)), sp("a\x00b")}
	}
	tr := &TargetResults{Results: []engine.QueryResult{{Columns: []string{"big", "nul"}, Rows: rows}}}
	raw := encodeResults(tr, 4096)
	if len(raw) > 4096 || bytes.Contains(raw, []byte(`\u0000`)) {
		t.Fatalf("encoded %d bytes: %.200s", len(raw), raw)
	}
	var back TargetResults
	if err := json.Unmarshal(raw, &back); err != nil || !back.Trimmed || !back.Results[0].Truncated || len(back.Results[0].Rows) == 0 {
		t.Fatalf("back = %+v, %v", back, err)
	}
}

func TestWriteCSVMergesColumns(t *testing.T) {
	svc, _, repo, _, _ := setup(t)
	user := uuid.New()
	run := &fleetdom.Run{ID: uuid.New(), CreatedBy: user}
	repo.runs[run.ID] = run
	enc := func(cols []string, rows ...[]*string) json.RawMessage {
		raw, _ := json.Marshal(TargetResults{Results: []engine.QueryResult{{Columns: cols, Rows: rows}}})
		return raw
	}
	repo.targets[run.ID] = []fleetdom.Target{
		{DatabaseName: "a", InstanceName: "s1", Results: enc([]string{"id", "name"}, []*string{sp("1"), sp("=cmd()")})},
		{DatabaseName: "b", InstanceName: "s2", Results: enc([]string{"id", "extra"}, []*string{sp("2"), nil})},
		{DatabaseName: "c", InstanceName: "s3", Status: fleetdom.TargetFailed},
	}
	var b bytes.Buffer
	if err := svc.WriteCSV(context.Background(), user, run.ID, &b); err != nil {
		t.Fatal(err)
	}
	want := "database,server,id,name,extra\na,s1,1,'=cmd(),\nb,s2,2,,\n"
	if b.String() != want {
		t.Fatalf("csv =\n%s\nwant\n%s", b.String(), want)
	}
	if err := svc.WriteCSV(context.Background(), uuid.New(), run.ID, &b); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("another user's run: %v", err)
	}
}
