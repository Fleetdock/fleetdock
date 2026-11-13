package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewWorkerIntervals(t *testing.T) {
	defaults := New(Deps{})
	if defaults.deps.JobPollInterval != 3*time.Second ||
		defaults.deps.AlertInterval != 15*time.Second ||
		defaults.deps.HealthCheckInterval != time.Minute {
		t.Fatalf("default intervals = %s, %s, %s", defaults.deps.JobPollInterval,
			defaults.deps.AlertInterval, defaults.deps.HealthCheckInterval)
	}

	custom := New(Deps{
		JobPollInterval:     2 * time.Second,
		AlertInterval:       10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
	})
	if custom.deps.JobPollInterval != 2*time.Second ||
		custom.deps.AlertInterval != 10*time.Second ||
		custom.deps.HealthCheckInterval != 30*time.Second {
		t.Fatalf("custom intervals = %s, %s, %s", custom.deps.JobPollInterval,
			custom.deps.AlertInterval, custom.deps.HealthCheckInterval)
	}
}

type probeCounter struct{ calls int }

func (p *probeCounter) ProbeAll(context.Context) (int, error) {
	p.calls++
	return 1, nil
}

func TestHealthPassUsesIndependentLock(t *testing.T) {
	probe := &probeCounter{}
	var healthLockCalls, housekeepingLockCalls int
	w := New(Deps{
		Probe: probe,
		HealthLock: func(context.Context) (func(), bool, error) {
			healthLockCalls++
			return func() {}, true, nil
		},
		HousekeepingLock: func(context.Context) (func(), bool, error) {
			housekeepingLockCalls++
			return func() {}, true, nil
		},
	})

	w.healthPass(context.Background())
	if healthLockCalls != 1 || housekeepingLockCalls != 0 || probe.calls != 1 {
		t.Fatalf("health lock calls = %d, housekeeping lock calls = %d, probes = %d",
			healthLockCalls, housekeepingLockCalls, probe.calls)
	}

	w.deps.HealthLock = func(context.Context) (func(), bool, error) {
		return nil, false, nil
	}
	w.healthPass(context.Background())
	if probe.calls != 1 {
		t.Fatalf("probe ran while another replica held the health lock: %d calls", probe.calls)
	}
}

// Housekeeping runs once across replicas: only the one holding the lock
// does the work, it always releases the lock, and a panic in a pass is
// contained instead of taking the API process down.
func TestWithLock(t *testing.T) {
	w := New(Deps{})
	ctx := context.Background()

	type outcome struct {
		ran, released bool
	}
	run := func(lock func(context.Context) (func(), bool, error), fn func(context.Context)) outcome {
		var o outcome
		wrapped := lock
		if lock != nil {
			wrapped = func(ctx context.Context) (func(), bool, error) {
				release, ok, err := lock(ctx)
				if release != nil {
					inner := release
					release = func() { o.released = true; inner() }
				}
				return release, ok, err
			}
		}
		w.withLock(ctx, wrapped, func(ctx context.Context) {
			o.ran = true
			fn(ctx)
		})
		return o
	}
	noop := func(context.Context) {}
	held := func(context.Context) (func(), bool, error) { return func() {}, true, nil }

	if o := run(held, noop); !o.ran || !o.released {
		t.Errorf("lock held: %+v, want ran and released", o)
	}
	if o := run(func(context.Context) (func(), bool, error) { return nil, false, nil }, noop); o.ran {
		t.Error("ran while another replica held the lock")
	}
	if o := run(func(context.Context) (func(), bool, error) { return nil, false, errors.New("db down") }, noop); o.ran {
		t.Error("ran although the lock could not be taken")
	}
	if o := run(nil, noop); !o.ran {
		t.Error("without a lock (single process) the pass must run")
	}
	if o := run(held, func(context.Context) { panic("boom") }); !o.released {
		t.Error("a panicking pass kept the lock")
	}
}
