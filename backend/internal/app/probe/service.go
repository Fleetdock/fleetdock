// Package probeapp periodically checks every instance the control plane holds
// credentials for: reachability, version and latency, and the databases on
// it. It keeps each instance's database list in sync on its own — new
// databases are added, vanished ones marked missing — replacing the manual
// "Test connection" and "Import DBs" steps, and fills the size / connection
// figures the dashboard shows.
package probeapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/app/dbtarget"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// Secrets is the secret store surface the probe needs.
type Secrets interface {
	Get(ctx context.Context, ref string) ([]byte, error)
}

// Reconciler applies a probe's view of an instance's databases.
type Reconciler interface {
	Reconcile(ctx context.Context, instanceID uuid.UUID, seen []databasedom.Observed, now time.Time) (databasedom.ReconcileResult, error)
}

// Emitter raises notification events (optional).
type Emitter interface {
	Emit(ctx context.Context, event, title, message, severity, resourceType string, resourceID uuid.UUID)
}

// AgentImporter asks a managed instance's agent to list its databases, for
// instances the control plane cannot reach directly (optional).
type AgentImporter interface {
	RequestImport(ctx context.Context, inst *instancedom.Instance) error
}

// Service runs probes.
type Service struct {
	instances       instancedom.Repository
	dbs             Reconciler
	servers         dbtarget.Servers
	secrets         Secrets
	notify          Emitter
	importer        AgentImporter
	timeout         time.Duration
	discoverTimeout time.Duration
	statsTimeout    time.Duration
	parallel        int

	// agentImportEvery bounds how often an unreachable managed instance's
	// agent is asked for its database list (each request is an operation).
	agentImportEvery time.Duration
	mu               sync.Mutex
	lastAgentImport  map[uuid.UUID]time.Time
	now              func() time.Time
}

// NewService wires the probe.
func NewService(instances instancedom.Repository, dbs Reconciler, servers dbtarget.Servers, secrets Secrets) *Service {
	return &Service{
		instances: instances, dbs: dbs, servers: servers, secrets: secrets,
		timeout: 10 * time.Second, discoverTimeout: 45 * time.Second, statsTimeout: 20 * time.Second,
		parallel:         8,
		agentImportEvery: time.Hour,
		lastAgentImport:  map[uuid.UUID]time.Time{},
		now:              time.Now,
	}
}

// SetNotifier enables notifications for databases going missing.
func (s *Service) SetNotifier(e Emitter) { s.notify = e }

// SetAgentImporter enables agent-side discovery for unreachable managed instances.
func (s *Service) SetAgentImporter(a AgentImporter) { s.importer = a }

// ProbeAll probes every instance with admin credentials, a few at a time, and
// returns how many were probed.
func (s *Service) ProbeAll(ctx context.Context) (int, error) {
	var all []*instancedom.Instance
	for offset := 0; ; offset += 100 {
		page, err := s.instances.List(ctx, instancedom.ListFilter{Limit: 100, Offset: offset})
		if err != nil {
			return 0, err
		}
		all = append(all, page.Items...)
		if len(page.Items) < 100 {
			break
		}
	}

	sem := make(chan struct{}, s.parallel)
	var wg sync.WaitGroup
	n := 0
	for _, inst := range all {
		if !probeable(inst) {
			continue
		}
		n++
		wg.Add(1)
		sem <- struct{}{}
		go func(inst *instancedom.Instance) {
			defer wg.Done()
			defer func() { <-sem }()
			s.Probe(ctx, inst)
		}(inst)
	}
	wg.Wait()
	return n, nil
}

func probeable(inst *instancedom.Instance) bool {
	return inst.HasCredentials() && inst.Status != instancedom.StatusProvisioning && inst.Status != instancedom.StatusDeleting
}

// ProbeOne checks one instance now ("Check now" in the dashboard) and returns
// its fresh health.
func (s *Service) ProbeOne(ctx context.Context, id string) (instancedom.Health, error) {
	iid, err := uuid.Parse(id)
	if err != nil {
		return instancedom.Health{}, apperr.Invalid("id", "id must be a valid UUID")
	}
	inst, err := s.instances.GetByID(ctx, iid)
	if err != nil {
		return instancedom.Health{}, err
	}
	if !inst.HasCredentials() {
		return instancedom.Health{}, apperr.Invalid("id", "add an admin login to this database server first")
	}
	return s.Probe(ctx, inst), nil
}

// Probe checks one instance, reconciles its databases and records the result.
// The reachability check and database discovery have separate deadlines: a
// slow size query must not cancel the import of the database list.
func (s *Service) Probe(ctx context.Context, inst *instancedom.Instance) instancedom.Health {
	h := instancedom.Health{CheckedAt: s.now().UTC()}
	pingCtx, cancel := context.WithTimeout(ctx, s.timeout)
	conn, eng, mon, err := s.connect(pingCtx, inst)
	if err == nil {
		start := time.Now()
		h.Version, err = eng.Ping(pingCtx, conn)
		h.LatencyMS = time.Since(start).Milliseconds()
	}
	cancel()
	switch {
	case err == nil:
		h.Status = instancedom.HealthHealthy
	case inst.Kind == instancedom.KindManaged:
		// The agent, not the control plane, is the managed instance's
		// lifeline; failing to reach it from here proves nothing.
		h.Status = instancedom.HealthUnknown
		h.Error = apperr.EngineMessage(err)
	default:
		h.Status = instancedom.HealthUnreachable
		h.Error = apperr.EngineMessage(err)
	}
	if err := s.instances.SetHealth(ctx, inst.ID, h); err != nil {
		slog.Warn("record instance health", "instance_id", inst.ID, "error", err.Error())
	}

	switch h.Status {
	case instancedom.HealthHealthy:
		s.discover(ctx, inst, eng, mon, conn)
	case instancedom.HealthUnknown:
		s.requestAgentImport(ctx, inst)
	}
	return h
}

// discover lists the instance's databases and reconciles them. The list is
// saved before usage stats are collected: those queries walk every table and,
// over an SSH tunnel, can outlast the health check.
func (s *Service) discover(ctx context.Context, inst *instancedom.Instance, eng engine.Client, mon engine.Monitor, conn engine.ConnParams) {
	listCtx, cancel := context.WithTimeout(ctx, s.discoverTimeout)
	listed, err := eng.ListDatabases(listCtx, conn)
	cancel()
	if err != nil {
		// Without a trustworthy list, reconciling could wrongly mark
		// everything missing; skip this round.
		slog.Warn("list databases", "instance_id", inst.ID, "error", err.Error())
		return
	}
	seen := observations(inst, listed, nil)
	if !s.reconcile(ctx, inst, seen) {
		return
	}
	if mon == nil || len(seen) == 0 {
		return
	}
	statsCtx, cancel := context.WithTimeout(ctx, s.statsTimeout)
	st, err := mon.DatabaseStats(statsCtx, conn)
	cancel()
	if err != nil {
		slog.Warn("database stats", "instance_id", inst.ID, "error", err.Error())
		return
	}
	stats := make(map[string]engine.DatabaseStat, len(st))
	for _, x := range st {
		stats[x.Name] = x
	}
	s.reconcile(ctx, inst, observations(inst, listed, stats))
}

// observations turns an engine listing into the rows reconcile stores.
// stats may be nil; sizes are filled in on a later pass.
func observations(inst *instancedom.Instance, listed []engine.DatabaseInfo, stats map[string]engine.DatabaseStat) []databasedom.Observed {
	seen := make([]databasedom.Observed, 0, len(listed))
	for _, d := range listed {
		if databasedom.IsScratchName(d.Name) {
			continue // a temporary backup restore, never registered
		}
		st := stats[d.Name]
		seen = append(seen, databasedom.Observed{
			Name: d.Name, Charset: d.Charset, Collation: d.Collation,
			System:    d.System || engine.IsSystemDatabase(string(inst.Engine), d.Name),
			SizeBytes: st.SizeBytes, Connections: st.Connections,
		})
	}
	return seen
}

// reconcile writes one observation pass. It reports whether the write landed.
func (s *Service) reconcile(ctx context.Context, inst *instancedom.Instance, seen []databasedom.Observed) bool {
	saveCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	res, err := s.dbs.Reconcile(saveCtx, inst.ID, seen, s.now())
	if err != nil {
		slog.Warn("reconcile databases", "instance_id", inst.ID, "error", err.Error())
		return false
	}
	if len(res.Added) > 0 {
		slog.Info("discovered databases", "instance_id", inst.ID, "names", strings.Join(res.Added, ","))
	}
	if len(res.Missing) > 0 && s.notify != nil {
		s.notify.Emit(ctx, "database.missing", "Database not found on its server",
			fmt.Sprintf("%s: %s can no longer be found. It may have been dropped or renamed outside Fleetdock.",
				inst.Name, strings.Join(res.Missing, ", ")),
			"warning", "instance", inst.ID)
	}
	return true
}

// requestAgentImport asks the agent of a managed instance the control plane
// cannot reach to report its databases, at most once per agentImportEvery.
func (s *Service) requestAgentImport(ctx context.Context, inst *instancedom.Instance) {
	if s.importer == nil || inst.Kind != instancedom.KindManaged {
		return
	}
	s.mu.Lock()
	last, ok := s.lastAgentImport[inst.ID]
	due := !ok || s.now().Sub(last) >= s.agentImportEvery
	if due {
		s.lastAgentImport[inst.ID] = s.now()
	}
	s.mu.Unlock()
	if !due {
		return
	}
	if err := s.importer.RequestImport(ctx, inst); err != nil {
		slog.Warn("request agent database import", "instance_id", inst.ID, "error", err.Error())
	}
}

func (s *Service) connect(ctx context.Context, inst *instancedom.Instance) (engine.ConnParams, engine.Client, engine.Monitor, error) {
	eng, err := engine.For(string(inst.Engine))
	if err != nil {
		return engine.ConnParams{}, nil, nil, err
	}
	mon, _ := eng.(engine.Monitor)
	host, err := dbtarget.Host(ctx, s.servers, inst, "instance")
	if err != nil {
		return engine.ConnParams{}, nil, nil, err
	}
	pw, err := s.secrets.Get(ctx, *inst.RootSecretRef)
	if err != nil {
		return engine.ConnParams{}, nil, nil, err
	}
	tunnel, err := dbtarget.Tunnel(ctx, s.secrets, s.instances, inst, "instance")
	if err != nil {
		return engine.ConnParams{}, nil, nil, err
	}
	return engine.ConnParams{Host: host, Port: inst.Port, User: *inst.Username, Password: string(pw),
		TLSMode: inst.TLSModeOrDefault(), SSH: tunnel}, eng, mon, nil
}

// AgentImportFunc adapts a function to AgentImporter.
type AgentImportFunc func(ctx context.Context, inst *instancedom.Instance) error

// RequestImport implements AgentImporter.
func (f AgentImportFunc) RequestImport(ctx context.Context, inst *instancedom.Instance) error {
	return f(ctx, inst)
}
