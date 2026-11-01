package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	fleetdom "github.com/Fleetdock/fleetdock/backend/internal/domain/fleet"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// FleetRepository is the Postgres adapter for fleetdom.Repository.
type FleetRepository struct {
	pool *pgxpool.Pool
}

// NewFleetRepository builds the repository.
func NewFleetRepository(pool *pgxpool.Pool) *FleetRepository { return &FleetRepository{pool: pool} }

var _ fleetdom.Repository = (*FleetRepository)(nil)

const fleetRunColumns = `r.id, r.created_by, r.sql, r.writes, r.status, r.concurrency, r.stop_on_error, r.canary,
	r.timeout_seconds, r.row_limit, r.token_scopes, r.total, r.succeeded, r.failed, r.skipped,
	(SELECT count(*) FROM fleet_run_targets t WHERE t.run_id = r.id AND t.status = 'running'),
	r.created_at, r.updated_at, r.finished_at`

func scanFleetRun(row pgx.Row) (*fleetdom.Run, error) {
	var r fleetdom.Run
	err := row.Scan(&r.ID, &r.CreatedBy, &r.SQL, &r.Writes, &r.Status, &r.Concurrency, &r.StopOnError, &r.Canary,
		&r.TimeoutSeconds, &r.RowLimit, &r.TokenScopes, &r.Total, &r.Succeeded, &r.Failed, &r.Skipped, &r.Running,
		&r.CreatedAt, &r.UpdatedAt, &r.FinishedAt)
	return &r, err
}

func (f *FleetRepository) Create(ctx context.Context, r *fleetdom.Run, targets []fleetdom.Target) error {
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return apperr.Internal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	r.Status, r.Total = fleetdom.RunRunning, len(targets)
	if err := tx.QueryRow(ctx, `
		INSERT INTO fleet_runs (created_by, sql, writes, status, concurrency, stop_on_error, canary, timeout_seconds,
		                        row_limit, token_scopes, total)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id, created_at, updated_at`,
		r.CreatedBy, r.SQL, r.Writes, r.Status, r.Concurrency, r.StopOnError, r.Canary, r.TimeoutSeconds, r.RowLimit,
		r.TokenScopes, r.Total).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return apperr.Internal(fmt.Errorf("create fleet run: %w", err))
	}
	rows := make([][]any, len(targets))
	for i, t := range targets {
		rows[i] = []any{r.ID, i, t.DatabaseID, t.DatabaseName, t.InstanceName, t.Engine, fleetdom.TargetPending}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"fleet_run_targets"},
		[]string{"run_id", "position", "database_id", "database_name", "instance_name", "engine", "status"},
		pgx.CopyFromRows(rows)); err != nil {
		return apperr.Internal(fmt.Errorf("create fleet run targets: %w", err))
	}
	return tx.Commit(ctx)
}

func (f *FleetRepository) Get(ctx context.Context, id uuid.UUID) (*fleetdom.Run, error) {
	r, err := scanFleetRun(f.pool.QueryRow(ctx, `SELECT `+fleetRunColumns+` FROM fleet_runs r WHERE r.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("fleet run not found")
	}
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("get fleet run: %w", err))
	}
	return r, nil
}

func (f *FleetRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit int) ([]fleetdom.Run, error) {
	rows, err := f.pool.Query(ctx, `SELECT `+fleetRunColumns+` FROM fleet_runs r WHERE r.created_by = $1
		ORDER BY r.created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list fleet runs: %w", err))
	}
	defer rows.Close()
	out := []fleetdom.Run{}
	for rows.Next() {
		r, err := scanFleetRun(rows)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (f *FleetRepository) Targets(ctx context.Context, runID uuid.UUID, withResults bool) ([]fleetdom.Target, error) {
	results := "NULL::jsonb"
	if withResults {
		results = "results"
	}
	rows, err := f.pool.Query(ctx, `SELECT run_id, position, database_id, database_name, instance_name, engine, status,
		`+results+`, rows_affected, error, started_at, finished_at
		FROM fleet_run_targets WHERE run_id = $1 ORDER BY position`, runID)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list fleet run targets: %w", err))
	}
	defer rows.Close()
	out := []fleetdom.Target{}
	for rows.Next() {
		var t fleetdom.Target
		var res []byte
		if err := rows.Scan(&t.RunID, &t.Position, &t.DatabaseID, &t.DatabaseName, &t.InstanceName, &t.Engine, &t.Status,
			&res, &t.RowsAffected, &t.Error, &t.StartedAt, &t.FinishedAt); err != nil {
			return nil, apperr.Internal(err)
		}
		if res != nil {
			t.Results = res
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (f *FleetRepository) Claim(ctx context.Context, owner string, max int) ([]fleetdom.Claim, error) {
	if max <= 0 {
		return nil, nil
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Locking the run row serialises claimers of one run, so its
	// concurrency holds across replicas; SKIP LOCKED lets them work on
	// other runs meanwhile.
	rows, err := tx.Query(ctx, `SELECT `+fleetRunColumns+` FROM fleet_runs r WHERE r.status = 'running'
		ORDER BY r.created_at LIMIT 20 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("claim fleet runs: %w", err))
	}
	var runs []fleetdom.Run
	for rows.Next() {
		r, err := scanFleetRun(rows)
		if err != nil {
			rows.Close()
			return nil, apperr.Internal(err)
		}
		runs = append(runs, *r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, apperr.Internal(err)
	}
	var out []fleetdom.Claim
	for _, r := range runs {
		left := max - len(out)
		if left <= 0 {
			break
		}
		free := r.Concurrency - r.Running
		cond := ""
		if r.Canary {
			// The canary runs alone, first.
			free, cond = 1-r.Running, " AND position = 0"
		}
		if free > left {
			free = left
		}
		if free <= 0 {
			continue
		}
		trows, err := tx.Query(ctx, `
			UPDATE fleet_run_targets SET status = 'running', claimed_by = $2, started_at = now()
			WHERE run_id = $1 AND position IN (
				SELECT position FROM fleet_run_targets WHERE run_id = $1 AND status = 'pending'`+cond+`
				ORDER BY position LIMIT $3)
			RETURNING position, database_id, database_name, instance_name, engine, started_at`, r.ID, owner, free)
		if err != nil {
			return nil, apperr.Internal(fmt.Errorf("claim fleet targets: %w", err))
		}
		for trows.Next() {
			t := fleetdom.Target{RunID: r.ID, Status: fleetdom.TargetRunning}
			if err := trows.Scan(&t.Position, &t.DatabaseID, &t.DatabaseName, &t.InstanceName, &t.Engine, &t.StartedAt); err != nil {
				trows.Close()
				return nil, apperr.Internal(err)
			}
			out = append(out, fleetdom.Claim{Run: r, Target: t})
		}
		trows.Close()
		if err := trows.Err(); err != nil {
			return nil, apperr.Internal(err)
		}
	}
	return out, tx.Commit(ctx)
}

// settle recomputes a run's totals and finishes it when no target is left
// pending or running.
func settle(ctx context.Context, tx pgx.Tx, runID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE fleet_runs r SET
			succeeded = c.s, failed = c.f, skipped = c.k, updated_at = now(),
			status = CASE WHEN c.open = 0 AND r.status IN ('running', 'paused', 'canceling') THEN
			              CASE WHEN r.status = 'canceling' THEN 'canceled' WHEN c.f > 0 THEN 'failed' ELSE 'succeeded' END
			         ELSE r.status END,
			finished_at = CASE WHEN c.open = 0 THEN coalesce(r.finished_at, now()) ELSE NULL END
		FROM (SELECT count(*) FILTER (WHERE status = 'succeeded') AS s,
		             count(*) FILTER (WHERE status IN ('failed', 'unknown')) AS f,
		             count(*) FILTER (WHERE status IN ('skipped', 'canceled')) AS k,
		             count(*) FILTER (WHERE status IN ('pending', 'running')) AS open
		      FROM fleet_run_targets WHERE run_id = $1) c
		WHERE r.id = $1`, runID)
	return err
}

func (f *FleetRepository) Complete(ctx context.Context, runID uuid.UUID, position int, o fleetdom.Outcome) error {
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return apperr.Internal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var stopOnError, canary bool
	var status string
	if err := tx.QueryRow(ctx, `SELECT status, stop_on_error, canary FROM fleet_runs WHERE id = $1 FOR UPDATE`, runID).
		Scan(&status, &stopOnError, &canary); err != nil {
		return apperr.Internal(fmt.Errorf("complete fleet target: %w", err))
	}
	var errMsg *string
	if o.Error != "" {
		errMsg = &o.Error
	}
	var results any
	if o.Results != nil {
		results = string(o.Results)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE fleet_run_targets SET status = $3, results = $4::jsonb, rows_affected = $5, error = $6, finished_at = now()
		WHERE run_id = $1 AND position = $2 AND status = 'running'`,
		runID, position, o.Status, results, o.RowsAffected, errMsg)
	if err != nil {
		return apperr.Internal(fmt.Errorf("complete fleet target: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return tx.Commit(ctx) // reaped meanwhile
	}
	if o.Status != fleetdom.TargetSucceeded && stopOnError && status == fleetdom.RunRunning {
		if _, err := tx.Exec(ctx, `UPDATE fleet_run_targets SET status = 'skipped' WHERE run_id = $1 AND status = 'pending'`, runID); err != nil {
			return apperr.Internal(err)
		}
	}
	if canary && position == 0 && status == fleetdom.RunRunning {
		// Wait for the user before the rest (unless nothing is left).
		if _, err := tx.Exec(ctx, `UPDATE fleet_runs SET status = 'paused' WHERE id = $1 AND EXISTS (
			SELECT 1 FROM fleet_run_targets WHERE run_id = $1 AND status = 'pending')`, runID); err != nil {
			return apperr.Internal(err)
		}
	}
	if err := settle(ctx, tx, runID); err != nil {
		return apperr.Internal(err)
	}
	return tx.Commit(ctx)
}

func (f *FleetRepository) Resume(ctx context.Context, runID uuid.UUID) error {
	tag, err := f.pool.Exec(ctx, `UPDATE fleet_runs SET status = 'running', canary = false, updated_at = now()
		WHERE id = $1 AND status = 'paused'`, runID)
	if err != nil {
		return apperr.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.Conflict("only a paused run can be resumed")
	}
	return nil
}

func (f *FleetRepository) Cancel(ctx context.Context, runID uuid.UUID) error {
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return apperr.Internal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE fleet_runs SET status = 'canceling', updated_at = now()
		WHERE id = $1 AND status IN ('running', 'paused')`, runID)
	if err != nil {
		return apperr.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.Conflict("the run is not running")
	}
	if _, err := tx.Exec(ctx, `UPDATE fleet_run_targets SET status = 'canceled' WHERE run_id = $1 AND status = 'pending'`, runID); err != nil {
		return apperr.Internal(err)
	}
	if err := settle(ctx, tx, runID); err != nil {
		return apperr.Internal(err)
	}
	return tx.Commit(ctx)
}

func (f *FleetRepository) Retry(ctx context.Context, runID uuid.UUID) error {
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return apperr.Internal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM fleet_runs WHERE id = $1 FOR UPDATE`, runID).Scan(&status); err != nil {
		return apperr.Internal(err)
	}
	if status != fleetdom.RunFailed && status != fleetdom.RunCanceled {
		return apperr.Conflict("only a failed or canceled run can be retried")
	}
	tag, err := tx.Exec(ctx, `
		UPDATE fleet_run_targets SET status = 'pending', results = NULL, rows_affected = NULL, error = NULL,
		       claimed_by = NULL, started_at = NULL, finished_at = NULL
		WHERE run_id = $1 AND status IN ('failed', 'skipped', 'canceled', 'unknown')`, runID)
	if err != nil {
		return apperr.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperr.Conflict("nothing to retry")
	}
	if _, err := tx.Exec(ctx, `UPDATE fleet_runs SET status = 'running', canary = false WHERE id = $1`, runID); err != nil {
		return apperr.Internal(err)
	}
	if err := settle(ctx, tx, runID); err != nil {
		return apperr.Internal(err)
	}
	return tx.Commit(ctx)
}

func (f *FleetRepository) Canceling(ctx context.Context, owner string) ([]fleetdom.Target, error) {
	rows, err := f.pool.Query(ctx, `
		SELECT t.run_id, t.position FROM fleet_run_targets t JOIN fleet_runs r ON r.id = t.run_id
		WHERE r.status = 'canceling' AND t.status = 'running' AND t.claimed_by = $1`, owner)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	defer rows.Close()
	var out []fleetdom.Target
	for rows.Next() {
		var t fleetdom.Target
		if err := rows.Scan(&t.RunID, &t.Position); err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (f *FleetRepository) ReapOrphans(ctx context.Context, cutoff time.Time) (int, error) {
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return 0, apperr.Internal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// A target still running well past its run's timeout lost its runner.
	rows, err := tx.Query(ctx, `
		UPDATE fleet_run_targets t SET status = 'unknown', finished_at = now(),
		       error = 'interrupted: the process running it stopped; it may or may not have completed'
		FROM fleet_runs r
		WHERE r.id = t.run_id AND t.status = 'running' AND t.started_at + make_interval(secs => r.timeout_seconds) < $1
		RETURNING t.run_id`, cutoff)
	if err != nil {
		return 0, apperr.Internal(err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, apperr.Internal(err)
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := settle(ctx, tx, id); err != nil {
			return 0, apperr.Internal(err)
		}
	}
	return len(ids), tx.Commit(ctx)
}

func (f *FleetRepository) PruneFinished(ctx context.Context, before time.Time) (int64, error) {
	tag, err := f.pool.Exec(ctx, `DELETE FROM fleet_runs WHERE finished_at IS NOT NULL AND finished_at < $1`, before)
	if err != nil {
		return 0, apperr.Internal(err)
	}
	return tag.RowsAffected(), nil
}
