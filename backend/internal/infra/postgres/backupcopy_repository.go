package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	backupcopydom "github.com/Fleetdock/fleetdock/backend/internal/domain/backupcopy"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// BackupCopyRepository is the Postgres adapter for backupcopydom.Repository.
type BackupCopyRepository struct {
	pool *pgxpool.Pool
}

// NewBackupCopyRepository builds the repository.
func NewBackupCopyRepository(pool *pgxpool.Pool) *BackupCopyRepository {
	return &BackupCopyRepository{pool: pool}
}

var _ backupcopydom.Repository = (*BackupCopyRepository)(nil)

func (r *BackupCopyRepository) Create(ctx context.Context, c *backupcopydom.Copy) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO backup_copies (database_id, backup_id, instance_id, scratch_name, status, ttl_seconds, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at, updated_at`,
		c.DatabaseID, c.BackupID, c.InstanceID, c.ScratchName, c.Status, int(c.TTL.Seconds()), c.CreatedBy,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return apperr.Conflict("this database already has a backup copy; discard it first")
	}
	if err != nil {
		return apperr.Internal(fmt.Errorf("create backup copy: %w", err))
	}
	return nil
}

const backupCopyColumns = `c.id, c.database_id, c.backup_id, c.instance_id, c.scratch_name, c.status, c.job_id,
	c.drop_job_id, c.drop_attempts, c.error, c.ttl_seconds, c.ready_at, c.expires_at, c.created_by, c.created_at,
	c.updated_at, b.created_at`

func scanBackupCopy(row pgx.Row) (*backupcopydom.Copy, error) {
	var c backupcopydom.Copy
	var ttl int
	err := row.Scan(&c.ID, &c.DatabaseID, &c.BackupID, &c.InstanceID, &c.ScratchName, &c.Status, &c.JobID,
		&c.DropJobID, &c.DropAttempts, &c.Error, &ttl, &c.ReadyAt, &c.ExpiresAt, &c.CreatedBy, &c.CreatedAt,
		&c.UpdatedAt, &c.BackupCreatedAt)
	c.TTL = time.Duration(ttl) * time.Second
	return &c, err
}

func (r *BackupCopyRepository) Get(ctx context.Context, id uuid.UUID) (*backupcopydom.Copy, error) {
	c, err := scanBackupCopy(r.pool.QueryRow(ctx, `SELECT `+backupCopyColumns+`
		FROM backup_copies c LEFT JOIN backups b ON b.id = c.backup_id WHERE c.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("backup copy not found")
	}
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("get backup copy: %w", err))
	}
	return c, nil
}

func (r *BackupCopyRepository) list(ctx context.Context, where string, args ...any) ([]backupcopydom.Copy, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+backupCopyColumns+`
		FROM backup_copies c LEFT JOIN backups b ON b.id = c.backup_id WHERE `+where, args...)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list backup copies: %w", err))
	}
	defer rows.Close()
	out := []backupcopydom.Copy{}
	for rows.Next() {
		c, err := scanBackupCopy(rows)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *BackupCopyRepository) ListByDatabase(ctx context.Context, databaseID uuid.UUID) ([]backupcopydom.Copy, error) {
	return r.list(ctx, `c.database_id = $1 AND c.status <> 'dropped' ORDER BY c.created_at DESC LIMIT 20`, databaseID)
}

func (r *BackupCopyRepository) ListDue(ctx context.Context, now, staleBefore time.Time) ([]backupcopydom.Copy, error) {
	return r.list(ctx, `(c.status = 'ready' AND c.expires_at < $1)
		OR (c.status = 'failed' AND c.drop_job_id IS NULL)
		OR (c.status = 'dropping' AND c.updated_at < $2 AND c.drop_attempts < $3)
		ORDER BY c.updated_at LIMIT 50`, now, staleBefore, backupcopydom.MaxDropAttempts)
}

func (r *BackupCopyRepository) exec(ctx context.Context, what, q string, args ...any) error {
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return apperr.Internal(fmt.Errorf("%s: %w", what, err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.Conflict("the backup copy changed state meanwhile")
	}
	return nil
}

func (r *BackupCopyRepository) SetJob(ctx context.Context, id, jobID uuid.UUID) error {
	return r.exec(ctx, "set copy job", `UPDATE backup_copies SET job_id = $2, updated_at = now() WHERE id = $1`, id, jobID)
}

func (r *BackupCopyRepository) MarkReady(ctx context.Context, id uuid.UUID, readyAt, expiresAt time.Time) error {
	return r.exec(ctx, "mark copy ready", `UPDATE backup_copies SET status = 'ready', ready_at = $2, expires_at = $3,
		updated_at = now() WHERE id = $1 AND status = 'restoring'`, id, readyAt, expiresAt)
}

func (r *BackupCopyRepository) MarkFailed(ctx context.Context, id uuid.UUID, msg string) error {
	return r.exec(ctx, "mark copy failed", `UPDATE backup_copies SET status = 'failed', error = $2, updated_at = now()
		WHERE id = $1 AND status IN ('restoring', 'ready')`, id, msg)
}

func (r *BackupCopyRepository) MarkDropping(ctx context.Context, id uuid.UUID, dropJobID *uuid.UUID) error {
	return r.exec(ctx, "mark copy dropping", `UPDATE backup_copies SET status = 'dropping', drop_job_id = coalesce($2, drop_job_id),
		drop_attempts = drop_attempts + CASE WHEN $2::uuid IS NULL THEN 0 ELSE 1 END, updated_at = now()
		WHERE id = $1 AND status <> 'dropped'`, id, dropJobID)
}

func (r *BackupCopyRepository) MarkDropped(ctx context.Context, id uuid.UUID) error {
	return r.exec(ctx, "mark copy dropped", `UPDATE backup_copies SET status = 'dropped', updated_at = now() WHERE id = $1`, id)
}

func (r *BackupCopyRepository) SetExpiry(ctx context.Context, id uuid.UUID, expiresAt time.Time) error {
	return r.exec(ctx, "extend copy", `UPDATE backup_copies SET expires_at = $2, updated_at = now()
		WHERE id = $1 AND status = 'ready'`, id, expiresAt)
}
