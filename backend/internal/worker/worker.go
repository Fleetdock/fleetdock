// Package worker is the control plane's in-process executor. It runs
// operations that target external instances (which no agent can reach from
// localhost) and housekeeping like flipping stale servers to offline.
package worker

import (
	"context"
	"log/slog"
	"time"

	agentapp "github.com/mariadb-cp/db-manager/backend/internal/app/agent"
	operationapp "github.com/mariadb-cp/db-manager/backend/internal/app/operation"
	jobdom "github.com/mariadb-cp/db-manager/backend/internal/domain/job"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/executor"
)

// Worker polls for control-plane operations and executes them.
type Worker struct {
	ops              *operationapp.Service
	agents           *agentapp.Service
	heartbeatTimeout time.Duration
}

// New builds a worker.
func New(ops *operationapp.Service, agents *agentapp.Service, heartbeatTimeout time.Duration) *Worker {
	return &Worker{ops: ops, agents: agents, heartbeatTimeout: heartbeatTimeout}
}

// Run blocks until ctx is canceled.
func (w *Worker) Run(ctx context.Context) {
	claim := time.NewTicker(3 * time.Second)
	defer claim.Stop()
	stale := time.NewTicker(time.Minute)
	defer stale.Stop()

	slog.Info("operations worker started")
	for {
		select {
		case <-ctx.Done():
			return
		case <-claim.C:
			// Drain all currently pending control-plane jobs, one at a time.
			for {
				if !w.runOne(ctx) {
					break
				}
			}
		case <-stale.C:
			if n, err := w.agents.MarkStale(ctx, w.heartbeatTimeout); err != nil {
				slog.Error("mark stale servers", "error", err.Error())
			} else if n > 0 {
				slog.Info("marked servers offline", "count", n)
			}
		}
	}
}

// runOne claims and executes a single job; it reports whether one was found.
func (w *Worker) runOne(ctx context.Context) bool {
	job, payload, err := w.ops.Claim(ctx, nil)
	if err != nil {
		slog.Error("claim operation", "error", err.Error())
		return false
	}
	if job == nil {
		return false
	}

	slog.Info("executing operation", "id", job.ID, "type", job.Type)
	execCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()

	result, err := executor.Execute(execCtx, string(job.Type), payload)
	if err != nil {
		msg := err.Error()
		if cerr := w.ops.Complete(ctx, job.ID, jobdom.StatusFailed, nil, &msg); cerr != nil {
			slog.Error("complete operation", "id", job.ID, "error", cerr.Error())
		}
		slog.Warn("operation failed", "id", job.ID, "type", job.Type, "error", msg)
		return true
	}
	if err := w.ops.Complete(ctx, job.ID, jobdom.StatusSucceeded, result, nil); err != nil {
		slog.Error("complete operation", "id", job.ID, "error", err.Error())
	}
	slog.Info("operation succeeded", "id", job.ID, "type", job.Type)
	return true
}
