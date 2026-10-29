// Package fleet models fleet queries: one SQL script run on many databases
// by a background runner, a few at a time, with per-database results.
package fleet

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Run statuses.
const (
	RunRunning   = "running"
	RunPaused    = "paused" // after the canary target, until resumed
	RunCanceling = "canceling"
	RunSucceeded = "succeeded"
	RunFailed    = "failed"
	RunCanceled  = "canceled"
)

// Target statuses.
const (
	TargetPending   = "pending"
	TargetRunning   = "running"
	TargetSucceeded = "succeeded"
	TargetFailed    = "failed"
	TargetSkipped   = "skipped"  // not run: an earlier target failed
	TargetCanceled  = "canceled" // not run, or stopped, because the run was canceled
	TargetUnknown   = "unknown"  // interrupted (the process running it died): outcome unknown
)

// Limits.
const (
	MaxTargets        = 500
	MaxConcurrency    = 16
	MaxTimeoutSeconds = 600
	MaxRowLimit       = 1000
	// MaxResultBytes bounds the results kept per target.
	MaxResultBytes = 1 << 20
)

// Run is one fleet query.
type Run struct {
	ID             uuid.UUID  `json:"id"`
	CreatedBy      uuid.UUID  `json:"created_by"`
	SQL            string     `json:"sql"`
	Writes         bool       `json:"writes"`
	Status         string     `json:"status"`
	Concurrency    int        `json:"concurrency"`
	StopOnError    bool       `json:"stop_on_error"`
	Canary         bool       `json:"canary"`
	TimeoutSeconds int        `json:"timeout_seconds"`
	RowLimit       int        `json:"row_limit"`
	TokenScopes    []string   `json:"-"`
	Total          int        `json:"total"`
	Succeeded      int        `json:"succeeded"`
	Failed         int        `json:"failed"`
	Skipped        int        `json:"skipped"`
	Running        int        `json:"running"` // computed
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

// Finished reports whether the run is over.
func (r *Run) Finished() bool {
	return r.Status == RunSucceeded || r.Status == RunFailed || r.Status == RunCanceled
}

// Target is one database of a run.
type Target struct {
	RunID        uuid.UUID       `json:"-"`
	Position     int             `json:"position"`
	DatabaseID   *uuid.UUID      `json:"database_id,omitempty"`
	DatabaseName string          `json:"database_name"`
	InstanceName string          `json:"instance_name"`
	Engine       string          `json:"engine"`
	Status       string          `json:"status"`
	Results      json.RawMessage `json:"results,omitempty"`
	RowsAffected *int64          `json:"rows_affected,omitempty"`
	Error        *string         `json:"error,omitempty"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	FinishedAt   *time.Time      `json:"finished_at,omitempty"`
}

// Claim is a target handed to a runner, with its run.
type Claim struct {
	Run    Run
	Target Target
}

// Outcome is how a target ended.
type Outcome struct {
	Status       string
	Results      json.RawMessage
	RowsAffected *int64
	Error        string
}

// Repository persists runs.
type Repository interface {
	Create(ctx context.Context, r *Run, targets []Target) error
	Get(ctx context.Context, id uuid.UUID) (*Run, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit int) ([]Run, error)
	// Targets lists a run's targets in order; withResults loads results.
	Targets(ctx context.Context, runID uuid.UUID, withResults bool) ([]Target, error)
	// Claim hands out up to max pending targets of running runs, keeping
	// each run within its concurrency (and to its first target while it is
	// a canary). Safe across replicas.
	Claim(ctx context.Context, owner string, max int) ([]Claim, error)
	// Complete records a target's outcome and updates the run: stop on
	// error, pause after the canary, finish when nothing is left.
	Complete(ctx context.Context, runID uuid.UUID, position int, o Outcome) error
	// Resume continues a paused run (the canary passed).
	Resume(ctx context.Context, runID uuid.UUID) error
	// Cancel stops a run: pending targets are canceled; running ones are
	// stopped by their runner (CancelingTargets).
	Cancel(ctx context.Context, runID uuid.UUID) error
	// Retry re-queues a finished run's failed, skipped, canceled and
	// unknown targets.
	Retry(ctx context.Context, runID uuid.UUID) error
	// Canceling lists the running targets of canceling runs claimed by owner.
	Canceling(ctx context.Context, owner string) ([]Target, error)
	// ReapOrphans marks running targets started before cutoff as unknown
	// (their runner died) and returns how many.
	ReapOrphans(ctx context.Context, startedBefore time.Time) (int, error)
	// PruneFinished deletes runs finished before cutoff.
	PruneFinished(ctx context.Context, before time.Time) (int64, error)
}
