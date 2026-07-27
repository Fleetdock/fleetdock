// Package worker is the control plane's in-process executor. It runs
// operations that target external instances (which no agent can reach from
// localhost) plus housekeeping: offline detection, scheduled backups, backup
// retention, metric-history pruning, alert evaluation and notification
// dispatch.
package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	agentapp "github.com/Fleetdock/fleetdock/backend/internal/app/agent"
	dbcredentialapp "github.com/Fleetdock/fleetdock/backend/internal/app/dbcredential"
	endpointapp "github.com/Fleetdock/fleetdock/backend/internal/app/endpoint"
	notificationapp "github.com/Fleetdock/fleetdock/backend/internal/app/notification"
	operationapp "github.com/Fleetdock/fleetdock/backend/internal/app/operation"
	scheduleapp "github.com/Fleetdock/fleetdock/backend/internal/app/schedule"
	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/executor"
)

// toJobLogs maps buffered executor log lines to domain log records for a job.
func toJobLogs(jobID uuid.UUID, lines []executor.LogLine) []jobdom.JobLog {
	out := make([]jobdom.JobLog, len(lines))
	for i, l := range lines {
		out[i] = jobdom.JobLog{JobID: jobID, Seq: l.Seq, Level: l.Level, Message: l.Message, CreatedAt: l.Time}
	}
	return out
}

// Deps are the collaborators the worker drives.
type Deps struct {
	Ops              *operationapp.Service
	Agents           *agentapp.Service
	Schedules        *scheduleapp.Service
	Notifications    *notificationapp.Service
	Endpoints        *endpointapp.Service
	Credentials      *dbcredentialapp.Service
	HeartbeatTimeout time.Duration
	MetricsRetention time.Duration
}

// Worker runs the control plane's background loops.
type Worker struct {
	deps Deps
}

// New builds a worker.
func New(deps Deps) *Worker { return &Worker{deps: deps} }

// Run blocks until ctx is canceled.
func (w *Worker) Run(ctx context.Context) {
	claim := time.NewTicker(3 * time.Second)
	defer claim.Stop()
	minute := time.NewTicker(time.Minute)
	defer minute.Stop()
	fast := time.NewTicker(15 * time.Second)
	defer fast.Stop()

	slog.Info("operations worker started")
	for {
		select {
		case <-ctx.Done():
			return
		case <-claim.C:
			// Drain all currently pending control-plane jobs.
			for w.runOne(ctx) {
			}
		case <-fast.C:
			w.evaluateAndDispatch(ctx)
		case <-minute.C:
			w.housekeeping(ctx)
		}
	}
}

// evaluateAndDispatch runs alert evaluation then delivers queued notifications.
func (w *Worker) evaluateAndDispatch(ctx context.Context) {
	if w.deps.Notifications == nil {
		return
	}
	if err := w.deps.Notifications.EvaluateAlerts(ctx); err != nil {
		slog.Error("evaluate alerts", "error", err.Error())
	}
	if _, err := w.deps.Notifications.DispatchPending(ctx); err != nil {
		slog.Error("dispatch notifications", "error", err.Error())
	}
}

// housekeeping runs the once-a-minute maintenance loops.
func (w *Worker) housekeeping(ctx context.Context) {
	// Offline detection + notification.
	if ids, err := w.deps.Agents.MarkStale(ctx, w.deps.HeartbeatTimeout); err != nil {
		slog.Error("mark stale servers", "error", err.Error())
	} else if len(ids) > 0 {
		slog.Info("marked servers offline", "count", len(ids))
		if w.deps.Notifications != nil {
			for _, id := range ids {
				w.deps.Notifications.Emit(ctx, "server.offline", "Server offline",
					"A server stopped sending heartbeats and was marked offline.", "warning", "server", id)
			}
		}
	}

	// Scheduled backups.
	if w.deps.Schedules != nil {
		if n, err := w.deps.Schedules.RunDue(ctx); err != nil {
			slog.Error("run due schedules", "error", err.Error())
		} else if n > 0 {
			slog.Info("triggered scheduled backups", "count", n)
		}
	}

	// Backup retention.
	if n, err := w.deps.Ops.PruneExpiredBackups(ctx, 100); err != nil {
		slog.Error("prune expired backups", "error", err.Error())
	} else if n > 0 {
		slog.Info("pruned expired backups", "count", n)
	}

	// Metric-history retention.
	if w.deps.MetricsRetention > 0 {
		cutoff := time.Now().Add(-w.deps.MetricsRetention)
		if n, err := w.deps.Agents.PruneHealthHistory(ctx, cutoff); err != nil {
			slog.Error("prune health history", "error", err.Error())
		} else if n > 0 {
			slog.Info("pruned health history", "count", n)
		}
	}

	// Gateway reconciliation drift recovery.
	if w.deps.Endpoints != nil {
		if err := w.deps.Endpoints.Reconcile(ctx); err != nil {
			slog.Error("gateway reconcile", "error", err.Error())
		}
	}

	// Expire application credentials.
	if w.deps.Credentials != nil {
		if n, err := w.deps.Credentials.ExpireDue(ctx); err != nil {
			slog.Error("expire credentials", "error", err.Error())
		} else if n > 0 {
			slog.Info("expired credentials", "count", n)
		}
	}
}

// runOne claims and executes a single job; it reports whether one was found.
func (w *Worker) runOne(ctx context.Context) bool {
	job, payload, err := w.deps.Ops.Claim(ctx, nil)
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

	if job.Type == jobdom.TypeReconcileGateway {
		var err error
		if w.deps.Endpoints != nil {
			err = w.deps.Endpoints.Reconcile(execCtx)
		}
		if err != nil {
			msg := err.Error()
			_ = w.deps.Ops.Complete(ctx, job.ID, jobdom.StatusFailed, nil, &msg)
			slog.Warn("operation failed", "id", job.ID, "type", job.Type, "error", msg)
			return true
		}
		_ = w.deps.Ops.Complete(ctx, job.ID, jobdom.StatusSucceeded, nil, nil)
		slog.Info("operation succeeded", "id", job.ID, "type", job.Type)
		return true
	}

	// In-process executor: flush each line straight to the DB (threshold 1) so
	// the detail page tails logs live. Flush errors are logged, never fatal.
	sink := executor.NewBufferedSink(1, func(lines []executor.LogLine) error {
		return w.deps.Ops.AppendLogs(ctx, job.ID, toJobLogs(job.ID, lines))
	}, func(err error) {
		slog.Warn("append operation logs", "id", job.ID, "error", err.Error())
	})

	result, err := executor.Execute(execCtx, string(job.Type), payload, sink)
	sink.Flush()
	if err != nil {
		msg := err.Error()
		if cerr := w.deps.Ops.Complete(ctx, job.ID, jobdom.StatusFailed, nil, &msg); cerr != nil {
			slog.Error("complete operation", "id", job.ID, "error", cerr.Error())
		}
		slog.Warn("operation failed", "id", job.ID, "type", job.Type, "error", msg)
		return true
	}
	if err := w.deps.Ops.Complete(ctx, job.ID, jobdom.StatusSucceeded, result, nil); err != nil {
		slog.Error("complete operation", "id", job.ID, "error", err.Error())
	}
	slog.Info("operation succeeded", "id", job.ID, "type", job.Type)
	return true
}
