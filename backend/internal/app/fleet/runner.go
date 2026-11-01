package fleetapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	dbadminapp "github.com/Fleetdock/fleetdock/backend/internal/app/dbadmin"
	fleetdom "github.com/Fleetdock/fleetdock/backend/internal/domain/fleet"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// maxRunResultBytes bounds what one run keeps in all; each target gets an
// equal share (at most fleetdom.MaxResultBytes).
const maxRunResultBytes = 64 << 20

// Runner works through queued fleet runs: it claims pending targets (within
// each run's concurrency and its own limit), runs the script on each as the
// run's creator — whose permissions are checked again first — and records
// the outcome. Several replicas can run one each.
type Runner struct {
	svc       *Service
	owner     string
	max       int
	retention time.Duration

	mu      sync.Mutex
	running map[string]*inflight
}

type inflight struct {
	cancel context.CancelFunc
	databaseID,
	queryID string
	userID   uuid.UUID
	canceled bool
}

// NewRunner builds a runner running at most max targets at once.
// retention is how long finished runs are kept (0 = forever).
func NewRunner(svc *Service, max int, retention time.Duration) *Runner {
	if max < 1 {
		max = 1
	}
	return &Runner{svc: svc, owner: uuid.NewString(), max: max, retention: retention, running: map[string]*inflight{}}
}

func targetKey(runID uuid.UUID, position int) string { return fmt.Sprintf("%s/%d", runID, position) }

// Run polls until ctx ends.
func (r *Runner) Run(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	var lastReap time.Time
	for {
		select {
		case <-ctx.Done():
			r.mu.Lock()
			for _, f := range r.running {
				f.cancel()
			}
			r.mu.Unlock()
			return
		case <-tick.C:
		}
		if time.Since(lastReap) > time.Minute {
			lastReap = time.Now()
			r.housekeeping(ctx)
		}
		r.stopCanceled(ctx)
		r.claim(ctx)
	}
}

func (r *Runner) housekeeping(ctx context.Context) {
	// Targets of a runner that died are given up on once well past their
	// timeout: their outcome is unknown, so they are never re-run silently.
	if n, err := r.svc.Repo.ReapOrphans(ctx, time.Now().Add(-5*time.Minute)); err != nil {
		slog.Error("reap fleet targets", "error", err.Error())
	} else if n > 0 {
		slog.Warn("fleet targets interrupted", "count", n)
	}
	if r.retention > 0 {
		if _, err := r.svc.Repo.PruneFinished(ctx, time.Now().Add(-r.retention)); err != nil {
			slog.Error("prune fleet runs", "error", err.Error())
		}
	}
}

// stopCanceled cancels this runner's targets of runs being canceled.
func (r *Runner) stopCanceled(ctx context.Context) {
	ts, err := r.svc.Repo.Canceling(ctx, r.owner)
	if err != nil {
		slog.Error("list canceled fleet targets", "error", err.Error())
		return
	}
	for _, t := range ts {
		r.mu.Lock()
		f := r.running[targetKey(t.RunID, t.Position)]
		if f != nil {
			f.canceled = true
		}
		r.mu.Unlock()
		if f == nil {
			continue
		}
		// Kill the statement on the server, then stop waiting for it.
		if f.queryID != "" {
			_ = r.svc.Queries.CancelQuery(ctx, f.databaseID, f.queryID, f.userID)
		}
		f.cancel()
	}
}

func (r *Runner) claim(ctx context.Context) {
	r.mu.Lock()
	free := r.max - len(r.running)
	r.mu.Unlock()
	if free <= 0 {
		return
	}
	claims, err := r.svc.Repo.Claim(ctx, r.owner, free)
	if err != nil {
		slog.Error("claim fleet targets", "error", err.Error())
		return
	}
	for _, c := range claims {
		tctx, cancel := context.WithCancel(ctx)
		f := &inflight{cancel: cancel, userID: c.Run.CreatedBy}
		r.mu.Lock()
		r.running[targetKey(c.Run.ID, c.Target.Position)] = f
		r.mu.Unlock()
		go r.execute(tctx, c, f)
	}
}

func (r *Runner) execute(ctx context.Context, c fleetdom.Claim, f *inflight) {
	key := targetKey(c.Run.ID, c.Target.Position)
	defer func() {
		if p := recover(); p != nil {
			slog.Error("fleet target panicked", "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
			r.complete(c, fleetdom.Outcome{Status: fleetdom.TargetFailed, Error: "internal error"})
		}
		r.mu.Lock()
		delete(r.running, key)
		r.mu.Unlock()
		f.cancel()
	}()
	o := r.svc.runTarget(ctx, c, func(databaseID, queryID string) {
		r.mu.Lock()
		f.databaseID, f.queryID = databaseID, queryID
		r.mu.Unlock()
	})
	r.mu.Lock()
	canceled := f.canceled
	r.mu.Unlock()
	if canceled && o.Status != fleetdom.TargetSucceeded {
		o.Status, o.Error = fleetdom.TargetCanceled, "canceled while running"
	}
	r.complete(c, o)
}

func (r *Runner) complete(c fleetdom.Claim, o fleetdom.Outcome) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := r.svc.Repo.Complete(ctx, c.Run.ID, c.Target.Position, o); err != nil {
		slog.Error("record fleet target", "run_id", c.Run.ID, "position", c.Target.Position, "error", err.Error())
	}
}

// runTarget runs the script on one target as the run's creator.
func (s *Service) runTarget(ctx context.Context, c fleetdom.Claim, started func(databaseID, queryID string)) fleetdom.Outcome {
	fail := func(msg string) fleetdom.Outcome { return fleetdom.Outcome{Status: fleetdom.TargetFailed, Error: msg} }
	if c.Target.DatabaseID == nil {
		return fail("the database was removed from Fleetdock")
	}
	dbID := *c.Target.DatabaseID
	p, err := s.Principals.PrincipalForUser(ctx, c.Run.CreatedBy, c.Run.TokenScopes)
	if err != nil {
		return fail("not run: " + apperr.EngineMessage(err))
	}
	if err := s.authorize(ctx, p, dbID, c.Run.Writes); err != nil {
		return fail("not run: " + apperr.EngineMessage(err))
	}
	queryID := uuid.NewString()
	started(dbID.String(), queryID)
	out, err := s.Queries.Query(ctx, dbadminapp.QueryInput{
		DatabaseID: dbID.String(),
		SQL:        c.Run.SQL,
		Limit:      c.Run.RowLimit,
		AllowWrite: c.Run.Writes,
		QueryID:    queryID,
		UserID:     c.Run.CreatedBy,
		Timeout:    time.Duration(c.Run.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return fleetdom.Outcome{Status: fleetdom.TargetCanceled, Error: "canceled while running"}
		}
		return fail(apperr.EngineMessage(err))
	}
	tr := TargetResults{Results: out.Results, Error: out.Error}
	var affected int64
	for _, r := range tr.Results {
		affected += r.RowsAffected
	}
	budget := maxRunResultBytes / max(c.Run.Total, 1)
	if budget > fleetdom.MaxResultBytes {
		budget = fleetdom.MaxResultBytes
	}
	raw := encodeResults(&tr, budget)
	o := fleetdom.Outcome{Status: fleetdom.TargetSucceeded, Results: raw, RowsAffected: &affected}
	if out.Error != nil {
		o.Status = fleetdom.TargetFailed
		o.Error = fmt.Sprintf("statement %d: %s", out.Error.Statement, out.Error.Message)
	}
	return o
}

// encodeResults serialises results for storage: NUL characters (which
// PostgreSQL text cannot hold) are replaced, and rows are dropped from the
// end until the JSON fits the budget.
func encodeResults(tr *TargetResults, budget int) json.RawMessage {
	for i := range tr.Results {
		res := &tr.Results[i]
		res.Sample, res.Notes = nil, nil
		for j, c := range res.Columns {
			res.Columns[j] = stripNUL(c)
		}
		for _, row := range res.Rows {
			for k, v := range row {
				if v != nil && strings.IndexByte(*v, 0) >= 0 {
					s := stripNUL(*v)
					row[k] = &s
				}
			}
		}
	}
	for {
		raw, err := json.Marshal(tr)
		if err != nil {
			return json.RawMessage(`{"results":[]}`)
		}
		if len(raw) <= budget {
			return raw
		}
		if !trimRows(tr.Results) {
			tr.Results = []engine.QueryResult{}
		}
		tr.Trimmed = true
	}
}

// trimRows halves the largest result's rows; false when none has rows left.
func trimRows(rs []engine.QueryResult) bool {
	best := -1
	for i, r := range rs {
		if len(r.Rows) > 0 && (best < 0 || len(r.Rows) > len(rs[best].Rows)) {
			best = i
		}
	}
	if best < 0 {
		return false
	}
	rs[best].Rows = rs[best].Rows[:len(rs[best].Rows)/2]
	rs[best].Truncated = true
	return true
}

func stripNUL(s string) string { return strings.ReplaceAll(s, "\x00", "�") }
