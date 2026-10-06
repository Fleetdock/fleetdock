package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// JobRepository is the Postgres adapter for jobdom.Repository.
type JobRepository struct {
	pool *pgxpool.Pool
}

// NewJobRepository builds a job repository.
func NewJobRepository(pool *pgxpool.Pool) *JobRepository { return &JobRepository{pool: pool} }

var _ jobdom.Repository = (*JobRepository)(nil)

const jobColumns = `
	id, type, resource_type, resource_id, status, server_id, params, result,
	error, progress, created_by, claimed_at, started_at, completed_at,
	created_at, updated_at, version`

// jobResourceName resolves a job's resource to its display name for reads.
const jobResourceName = `
	COALESCE(CASE resource_type
		WHEN 'database' THEN (SELECT name FROM databases WHERE id = jobs.resource_id)
		WHEN 'instance' THEN (SELECT name FROM instances WHERE id = jobs.resource_id)
		WHEN 'server'   THEN (SELECT name FROM servers WHERE id = jobs.resource_id)
		WHEN 'backup'   THEN (SELECT d.name FROM backups b JOIN databases d ON d.id = b.database_id
		                      WHERE b.id = jobs.resource_id)
	END, '')`

func (r *JobRepository) Create(ctx context.Context, j *jobdom.Job) error {
	params := j.Params
	if params == nil {
		params = json.RawMessage(`{}`)
	}
	const q = `
		INSERT INTO jobs (id, type, resource_type, resource_id, status, server_id, params, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
		RETURNING created_at, updated_at, version`
	err := r.pool.QueryRow(ctx, q,
		j.ID, string(j.Type), j.ResourceType, j.ResourceID, string(j.Status), j.ServerID, string(params), j.CreatedBy,
	).Scan(&j.CreatedAt, &j.UpdatedAt, &j.Version)
	if err != nil {
		return apperr.Internal(fmt.Errorf("insert job: %w", err))
	}
	return nil
}

func (r *JobRepository) GetByID(ctx context.Context, id uuid.UUID) (*jobdom.Job, error) {
	q := `SELECT ` + jobColumns + `, ` + jobResourceName + ` FROM jobs WHERE id = $1`
	j, err := scanJob(r.pool.QueryRow(ctx, q, id), true)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("operation not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get job: %w", err))
	}
	return j, nil
}

func (r *JobRepository) List(ctx context.Context, f jobdom.ListFilter) (jobdom.Page, error) {
	conds := []string{"true"}
	args := make([]any, 0, 5)
	if f.Status != nil {
		args = append(args, string(*f.Status))
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.Type != nil {
		args = append(args, string(*f.Type))
		conds = append(conds, fmt.Sprintf("type = $%d", len(args)))
	}
	if f.ResourceType != "" {
		args = append(args, f.ResourceType)
		conds = append(conds, fmt.Sprintf("resource_type = $%d", len(args)))
	}
	if f.ResourceID != nil {
		args = append(args, *f.ResourceID)
		conds = append(conds, fmt.Sprintf("resource_id = $%d", len(args)))
	}
	if f.CreatedBy != nil {
		args = append(args, *f.CreatedBy)
		conds = append(conds, fmt.Sprintf("created_by = $%d", len(args)))
	}
	args = append(args, f.Limit)
	limitPos := len(args)
	args = append(args, f.Offset)
	offsetPos := len(args)

	q := fmt.Sprintf(
		`SELECT %s, %s, count(*) OVER() AS total FROM jobs WHERE %s
		 ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		jobColumns, jobResourceName, join(conds), limitPos, offsetPos)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return jobdom.Page{}, apperr.Internal(fmt.Errorf("list jobs: %w", err))
	}
	defer rows.Close()

	items := make([]*jobdom.Job, 0)
	total := 0
	for rows.Next() {
		j, t, err := scanJobWithTotal(rows)
		if err != nil {
			return jobdom.Page{}, apperr.Internal(fmt.Errorf("scan job: %w", err))
		}
		items = append(items, j)
		total = t
	}
	if err := rows.Err(); err != nil {
		return jobdom.Page{}, apperr.Internal(err)
	}
	return jobdom.Page{Items: items, Total: total}, nil
}

func (r *JobRepository) ClaimNext(ctx context.Context, serverID *uuid.UUID) (*jobdom.Job, error) {
	cond := "server_id IS NULL"
	args := []any{}
	if serverID != nil {
		cond = "server_id = $1"
		args = append(args, *serverID)
	}
	q := fmt.Sprintf(`
		UPDATE jobs SET status = 'running', claimed_at = now(), started_at = now(), version = version + 1
		WHERE id = (
			SELECT id FROM jobs
			WHERE status = 'pending' AND %s
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING `+jobColumns, cond)
	j, err := scanJob(r.pool.QueryRow(ctx, q, args...), false)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // nothing to do
		}
		return nil, apperr.Internal(fmt.Errorf("claim job: %w", err))
	}
	return j, nil
}

func (r *JobRepository) Complete(ctx context.Context, id uuid.UUID, status jobdom.Status, result json.RawMessage, errMsg *string) error {
	var res any
	if result != nil {
		res = string(result)
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE jobs SET status = $2, result = $3::jsonb, error = $4,
			progress = CASE WHEN $2 = 'succeeded' THEN 100 ELSE progress END,
			completed_at = now(), version = version + 1
		WHERE id = $1 AND status IN ('pending','running')`,
		id, string(status), res, errMsg)
	if err != nil {
		return apperr.Internal(fmt.Errorf("complete job: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.Conflict("operation is already finalized")
	}
	return nil
}

func (r *JobRepository) UpdateProgress(ctx context.Context, id uuid.UUID, progress int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE jobs SET progress = $2, version = version + 1 WHERE id = $1 AND status = 'running'`,
		id, progress)
	if err != nil {
		return apperr.Internal(fmt.Errorf("update job progress: %w", err))
	}
	return nil
}

func (r *JobRepository) AppendLogs(ctx context.Context, jobID uuid.UUID, lines []jobdom.JobLog) error {
	if len(lines) == 0 {
		return nil
	}
	rows := make([][]any, 0, len(lines))
	for _, l := range lines {
		level := l.Level
		if level == "" {
			level = "info"
		}
		rows = append(rows, []any{jobID, l.Seq, level, l.Message})
	}
	_, err := r.pool.CopyFrom(ctx,
		pgx.Identifier{"job_logs"},
		[]string{"job_id", "seq", "level", "message"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return apperr.Internal(fmt.Errorf("append job logs: %w", err))
	}
	return nil
}

func (r *JobRepository) ListLogs(ctx context.Context, jobID uuid.UUID, afterSeq, limit int) ([]jobdom.JobLog, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT seq, level, message, created_at FROM job_logs
		WHERE job_id = $1 AND seq > $2
		ORDER BY seq
		LIMIT $3`, jobID, afterSeq, limit)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list job logs: %w", err))
	}
	defer rows.Close()

	logs := make([]jobdom.JobLog, 0)
	for rows.Next() {
		l := jobdom.JobLog{JobID: jobID}
		if err := rows.Scan(&l.Seq, &l.Level, &l.Message, &l.CreatedAt); err != nil {
			return nil, apperr.Internal(fmt.Errorf("scan job log: %w", err))
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, apperr.Internal(err)
	}
	return logs, nil
}

// scanJob scans jobColumns, followed by the resource name when named is set.
func scanJob(row rowScanner, named bool) (*jobdom.Job, error) {
	j, dest := jobDest()
	if named {
		dest = append(dest, &j.ResourceName)
	}
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return j.finish(), nil
}

// scanJobWithTotal scans jobColumns, the resource name and a window count.
func scanJobWithTotal(row rowScanner) (*jobdom.Job, int, error) {
	var total int
	j, dest := jobDest()
	dest = append(dest, &j.ResourceName, &total)
	if err := row.Scan(dest...); err != nil {
		return nil, 0, err
	}
	return j.finish(), total, nil
}

type scannedJob struct {
	jobdom.Job
	typ, status string
	params, res []byte
}

func jobDest() (*scannedJob, []any) {
	j := &scannedJob{}
	return j, []any{
		&j.ID, &j.typ, &j.ResourceType, &j.ResourceID, &j.status, &j.ServerID, &j.params, &j.res,
		&j.Error, &j.Progress, &j.CreatedBy, &j.ClaimedAt, &j.StartedAt, &j.CompletedAt,
		&j.CreatedAt, &j.UpdatedAt, &j.Version,
	}
}

func (j *scannedJob) finish() *jobdom.Job {
	j.Type = jobdom.Type(j.typ)
	j.Status = jobdom.Status(j.status)
	j.Params = j.params
	j.Result = j.res
	return &j.Job
}

func (r *JobRepository) ListStuck(ctx context.Context, claimedBefore time.Time) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id FROM jobs
		WHERE status = 'running' AND server_id IS NULL AND claimed_at < $1
		ORDER BY claimed_at LIMIT 100`, claimedBefore)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list stuck jobs: %w", err))
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, apperr.Internal(err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
