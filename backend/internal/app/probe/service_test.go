package probeapp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

type healthRepo struct {
	instancedom.Repository
	got map[uuid.UUID]instancedom.Health
}

func (r *healthRepo) SetHealth(_ context.Context, id uuid.UUID, h instancedom.Health) error {
	r.got[id] = h
	return nil
}

type fakeReconciler struct {
	calls  [][]databasedom.Observed
	result databasedom.ReconcileResult
}

func (f *fakeReconciler) Reconcile(_ context.Context, _ uuid.UUID, seen []databasedom.Observed, _ time.Time) (databasedom.ReconcileResult, error) {
	f.calls = append(f.calls, seen)
	return f.result, nil
}

type addrServers struct{}

func (addrServers) GetByID(context.Context, uuid.UUID) (*serverdom.Server, error) {
	a := "127.0.0.1"
	return &serverdom.Server{Address: &a}, nil
}

type pw struct{}

func (pw) Get(context.Context, string) ([]byte, error) { return []byte("x"), nil }

type recorder struct{ events []string }

func (r *recorder) Emit(_ context.Context, event, _, _, _, _ string, _ uuid.UUID) {
	r.events = append(r.events, event)
}

// fakeEngine answers like a reachable server with a fixed database list.
type fakeEngine struct {
	engine.Client
	listErr error
}

func (fakeEngine) Ping(context.Context, engine.ConnParams) (string, error) { return "fake 1.0", nil }
func (f fakeEngine) ListDatabases(context.Context, engine.ConnParams) ([]engine.DatabaseInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	// fdv_…: a backup verification's temporary database, never registered.
	return []engine.DatabaseInfo{{Name: "app", Charset: "utf8"}, {Name: "fdv_0123abcd456789ef"}, {Name: "logs"}}, nil
}

func newInstance(engineName string, kind instancedom.Kind, port int) *instancedom.Instance {
	user, ref, host := "root", "r", "127.0.0.1"
	sid := uuid.New()
	inst := &instancedom.Instance{ID: uuid.New(), Name: "primary", Engine: instancedom.Engine(engineName), Kind: kind,
		Port: port, Username: &user, RootSecretRef: &ref}
	if kind == instancedom.KindExternal {
		inst.Host = &host
	} else {
		inst.ServerID = &sid
	}
	return inst
}

func TestProbe_HealthyReconcilesAndNotifiesMissing(t *testing.T) {
	engine.Register("probe-fake", fakeEngine{})
	repo := &healthRepo{got: map[uuid.UUID]instancedom.Health{}}
	rec := &fakeReconciler{result: databasedom.ReconcileResult{Missing: []string{"old"}}}
	svc := NewService(repo, rec, addrServers{}, pw{})
	notes := &recorder{}
	svc.SetNotifier(notes)

	inst := newInstance("probe-fake", instancedom.KindExternal, 5432)
	h := svc.Probe(context.Background(), inst)
	if h.Status != instancedom.HealthHealthy {
		t.Fatalf("health = %+v", h)
	}
	if len(rec.calls) != 1 || len(rec.calls[0]) != 2 || rec.calls[0][0].Name != "app" || rec.calls[0][0].Charset != "utf8" {
		t.Fatalf("reconcile calls = %+v", rec.calls)
	}
	if len(notes.events) != 1 || notes.events[0] != "database.missing" {
		t.Errorf("events = %v, want one database.missing", notes.events)
	}
}

func TestProbe_ListFailureDoesNotReconcile(t *testing.T) {
	engine.Register("probe-fake-broken", fakeEngine{listErr: errors.New("permission denied")})
	rec := &fakeReconciler{}
	svc := NewService(&healthRepo{got: map[uuid.UUID]instancedom.Health{}}, rec, addrServers{}, pw{})
	svc.Probe(context.Background(), newInstance("probe-fake-broken", instancedom.KindExternal, 5432))
	if len(rec.calls) != 0 {
		t.Error("an unreliable listing must not be reconciled (everything would look missing)")
	}
}

func TestProbe_FailureStatusDependsOnKind(t *testing.T) {
	repo := &healthRepo{got: map[uuid.UUID]instancedom.Health{}}
	svc := NewService(repo, &fakeReconciler{}, addrServers{}, pw{})
	// Port 1 on loopback refuses immediately.
	external := newInstance(string(instancedom.EngineMariaDB), instancedom.KindExternal, 1)
	managed := newInstance(string(instancedom.EnginePostgres), instancedom.KindManaged, 1)

	if h := svc.Probe(context.Background(), external); h.Status != instancedom.HealthUnreachable || h.Error == "" {
		t.Errorf("external = %+v, want unreachable with an error", h)
	}
	if h := svc.Probe(context.Background(), managed); h.Status != instancedom.HealthUnknown {
		t.Errorf("managed = %+v, want unknown (agent is the source of truth)", h)
	}
}

func TestProbe_UnreachableManagedAsksAgentAtMostHourly(t *testing.T) {
	svc := NewService(&healthRepo{got: map[uuid.UUID]instancedom.Health{}}, &fakeReconciler{}, addrServers{}, pw{})
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return clock }
	requests := 0
	svc.SetAgentImporter(AgentImportFunc(func(context.Context, *instancedom.Instance) error {
		requests++
		return nil
	}))
	managed := newInstance(string(instancedom.EnginePostgres), instancedom.KindManaged, 1)

	svc.Probe(context.Background(), managed)
	clock = clock.Add(30 * time.Minute)
	svc.Probe(context.Background(), managed)
	if requests != 1 {
		t.Fatalf("requests after 30 min = %d, want 1", requests)
	}
	clock = clock.Add(31 * time.Minute)
	svc.Probe(context.Background(), managed)
	if requests != 2 {
		t.Errorf("requests after 61 min = %d, want 2", requests)
	}
}

// blockingStats lists databases immediately and holds DatabaseStats until its
// context ends, standing in for a size query that outlasts the health check.
type blockingStats struct {
	fakeEngine
	note func(string)
}

func (blockingStats) Processes(context.Context, engine.ConnParams) ([]engine.Process, error) {
	return nil, nil
}
func (blockingStats) KillProcess(context.Context, engine.ConnParams, int64, bool) error {
	return nil
}
func (blockingStats) ServerStatus(context.Context, engine.ConnParams) ([]engine.Setting, error) {
	return nil, nil
}
func (blockingStats) Variables(context.Context, engine.ConnParams) ([]engine.Setting, error) {
	return nil, nil
}
func (b blockingStats) DatabaseStats(ctx context.Context, _ engine.ConnParams) ([]engine.DatabaseStat, error) {
	b.note("stats")
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestProbe_SavesDatabaseListBeforeSlowStats(t *testing.T) {
	var mu sync.Mutex
	var order []string
	note := func(step string) {
		mu.Lock()
		order = append(order, step)
		mu.Unlock()
	}
	engine.Register("probe-slow-stats", blockingStats{note: note})
	rec := &fakeReconciler{}
	svc := NewService(&healthRepo{got: map[uuid.UUID]instancedom.Health{}}, &notifyingReconciler{
		fakeReconciler: rec,
		note:           note,
	}, addrServers{}, pw{})
	svc.statsTimeout = 30 * time.Millisecond

	svc.Probe(context.Background(), newInstance("probe-slow-stats", instancedom.KindExternal, 5432))

	mu.Lock()
	defer mu.Unlock()
	if len(order) < 2 || order[0] != "save" || order[1] != "stats" {
		t.Fatalf("order = %v, want save before stats", order)
	}
	if len(rec.calls) == 0 || len(rec.calls[0]) != 2 || rec.calls[0][0].Name != "app" {
		t.Fatalf("first reconcile = %+v", rec.calls)
	}
}

// notifyingReconciler records when the database list is written.
type notifyingReconciler struct {
	*fakeReconciler
	note func(string)
	once bool
}

func (n *notifyingReconciler) Reconcile(ctx context.Context, id uuid.UUID, seen []databasedom.Observed, now time.Time) (databasedom.ReconcileResult, error) {
	if !n.once {
		n.once = true
		n.note("save")
	}
	return n.fakeReconciler.Reconcile(ctx, id, seen, now)
}
