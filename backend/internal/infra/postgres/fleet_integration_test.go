//go:build integration

package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	fleetdom "github.com/Fleetdock/fleetdock/backend/internal/domain/fleet"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

func TestIntegrationFleetRuns(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, scratchDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var user uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, name) VALUES ('f@example.com', 'u') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	repo := NewFleetRepository(pool)
	mk := func(n, concurrency int, canary, stop bool) *fleetdom.Run {
		t.Helper()
		r := &fleetdom.Run{CreatedBy: user, SQL: "SELECT 1", Concurrency: concurrency, Canary: canary, StopOnError: stop,
			TimeoutSeconds: 60, RowLimit: 10}
		targets := make([]fleetdom.Target, n)
		for i := range targets {
			targets[i] = fleetdom.Target{DatabaseName: "db", InstanceName: "i", Engine: "postgres"}
		}
		if err := repo.Create(ctx, r, targets); err != nil {
			t.Fatal(err)
		}
		return r
	}
	get := func(id uuid.UUID) *fleetdom.Run {
		t.Helper()
		r, err := repo.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	ok := fleetdom.Outcome{Status: fleetdom.TargetSucceeded, Results: []byte(`{"results":[]}`)}
	bad := fleetdom.Outcome{Status: fleetdom.TargetFailed, Error: "boom"}

	// Concurrency holds across concurrent claimers.
	r1 := mk(5, 2, false, false)
	var mu sync.Mutex
	var claimed []fleetdom.Claim
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := repo.Claim(ctx, uuid.NewString(), 10)
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			claimed = append(claimed, c...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(claimed) != 2 {
		t.Fatalf("claimed %d targets with concurrency 2", len(claimed))
	}
	for _, c := range claimed {
		if err := repo.Complete(ctx, r1.ID, c.Target.Position, ok); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		c, err := repo.Claim(ctx, "w", 1)
		if err != nil || len(c) != 1 {
			t.Fatalf("claim %d: %v %v", i, c, err)
		}
		if err := repo.Complete(ctx, r1.ID, c[0].Target.Position, ok); err != nil {
			t.Fatal(err)
		}
	}
	if r := get(r1.ID); r.Status != fleetdom.RunSucceeded || r.Succeeded != 5 || r.FinishedAt == nil {
		t.Fatalf("run 1 = %+v", r)
	}

	// Canary: only the first target, then a pause until resumed.
	r2 := mk(3, 3, true, true)
	c, _ := repo.Claim(ctx, "w", 10)
	if len(c) != 1 || c[0].Target.Position != 0 {
		t.Fatalf("canary claim = %+v", c)
	}
	_ = repo.Complete(ctx, r2.ID, 0, ok)
	if r := get(r2.ID); r.Status != fleetdom.RunPaused {
		t.Fatalf("after canary: %s", r.Status)
	}
	if c, _ := repo.Claim(ctx, "w", 10); len(c) != 0 {
		t.Fatal("a paused run handed out work")
	}
	if err := repo.Resume(ctx, r2.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Resume(ctx, r2.ID); apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("second resume: %v", err)
	}
	c, _ = repo.Claim(ctx, "w", 10)
	if len(c) != 2 {
		t.Fatalf("after resume claimed %d", len(c))
	}
	// Stop on error: the failure skips nothing left (both claimed); the run fails.
	_ = repo.Complete(ctx, r2.ID, c[0].Target.Position, bad)
	if r := get(r2.ID); r.Status != fleetdom.RunRunning || r.Running != 1 {
		t.Fatalf("mid-failure: %+v", r)
	}
	_ = repo.Complete(ctx, r2.ID, c[1].Target.Position, ok)
	if r := get(r2.ID); r.Status != fleetdom.RunFailed || r.Failed != 1 || r.Succeeded != 2 {
		t.Fatalf("run 2 = %+v", r)
	}

	// Retry re-queues only what did not succeed.
	if err := repo.Retry(ctx, r2.ID); err != nil {
		t.Fatal(err)
	}
	c, _ = repo.Claim(ctx, "w", 10)
	if len(c) != 1 || get(r2.ID).Status != fleetdom.RunRunning {
		t.Fatalf("retry claimed %d", len(c))
	}
	_ = repo.Complete(ctx, r2.ID, c[0].Target.Position, ok)
	if r := get(r2.ID); r.Status != fleetdom.RunSucceeded || r.Succeeded != 3 {
		t.Fatalf("after retry: %+v", r)
	}

	// Stop on error skips what is left; cancel stops the rest.
	r3 := mk(4, 1, false, true)
	c, _ = repo.Claim(ctx, "w", 10)
	_ = repo.Complete(ctx, r3.ID, c[0].Target.Position, bad)
	if r := get(r3.ID); r.Status != fleetdom.RunFailed || r.Skipped != 3 {
		t.Fatalf("run 3 = %+v", r)
	}
	r4 := mk(3, 1, false, false)
	c, _ = repo.Claim(ctx, "runner-a", 10)
	if err := repo.Cancel(ctx, r4.ID); err != nil {
		t.Fatal(err)
	}
	if r := get(r4.ID); r.Status != fleetdom.RunCanceling {
		t.Fatalf("cancel with one running: %s", r.Status)
	}
	if ts, _ := repo.Canceling(ctx, "runner-a"); len(ts) != 1 {
		t.Fatalf("canceling targets = %v", ts)
	}
	_ = repo.Complete(ctx, r4.ID, c[0].Target.Position, fleetdom.Outcome{Status: fleetdom.TargetCanceled})
	if r := get(r4.ID); r.Status != fleetdom.RunCanceled || r.Skipped != 3 {
		t.Fatalf("run 4 = %+v", r)
	}

	// A target whose runner died is marked unknown, never re-run.
	r5 := mk(1, 1, false, false)
	c, _ = repo.Claim(ctx, "dead", 10)
	if n, err := repo.ReapOrphans(ctx, time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("reap = %d, %v", n, err)
	}
	_ = repo.Complete(ctx, r5.ID, c[0].Target.Position, ok) // too late: ignored
	ts, _ := repo.Targets(ctx, r5.ID, true)
	if ts[0].Status != fleetdom.TargetUnknown || get(r5.ID).Status != fleetdom.RunFailed {
		t.Fatalf("reaped target = %+v", ts[0])
	}

	if runs, _ := repo.ListByUser(ctx, user, 10); len(runs) != 5 {
		t.Fatalf("listed %d runs", len(runs))
	}
	if n, err := repo.PruneFinished(ctx, time.Now().Add(time.Minute)); err != nil || n != 5 {
		t.Fatalf("prune = %d, %v", n, err)
	}
}
