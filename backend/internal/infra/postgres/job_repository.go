package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	q := `SELECT ` + jobColumns + ` FROM jobs WHERE id = $1`
	j, err := scanJob(r.pool.QueryRow(ctx, q, id))
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
		`SELECT %s, count(*) OVER() AS total FROM jobs WHERE %s
		 ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		jobColumns, join(conds), limitPos, offsetPos)

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
	j, err := scanJob(r.pool.QueryRow(ctx, q, args...))
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

func scanJob(row rowScanner) (*jobdom.Job, error) {
	var (
		j           jobdom.Job
		typ, status string
		params, res []byte
	)
	if err := row.Scan(
		&j.ID, &typ, &j.ResourceType, &j.ResourceID, &status, &j.ServerID, &params, &res,
		&j.Error, &j.Progress, &j.CreatedBy, &j.ClaimedAt, &j.StartedAt, &j.CompletedAt,
		&j.CreatedAt, &j.UpdatedAt, &j.Version,
	); err != nil {
		return nil, err
	}
	j.Type = jobdom.Type(typ)
	j.Status = jobdom.Status(status)
	j.Params = params
	j.Result = res
	return &j, nil
}

func scanJobWithTotal(row rowScanner) (*jobdom.Job, int, error) {
	var (
		j           jobdom.Job
		typ, status string
		params, res []byte
		total       int
	)
	if err := row.Scan(
		&j.ID, &typ, &j.ResourceType, &j.ResourceID, &status, &j.ServerID, &params, &res,
		&j.Error, &j.Progress, &j.CreatedBy, &j.ClaimedAt, &j.StartedAt, &j.CompletedAt,
		&j.CreatedAt, &j.UpdatedAt, &j.Version, &total,
	); err != nil {
		return nil, 0, err
	}
	j.Type = jobdom.Type(typ)
	j.Status = jobdom.Status(status)
	j.Params = params
	j.Result = res
	return &j, total, nil
}
