package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	backupdom "github.com/TajBrains/db-manager/backend/internal/domain/backup"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// BackupRepository is the Postgres adapter for backupdom.Repository.
type BackupRepository struct {
	pool *pgxpool.Pool
}

// NewBackupRepository builds a backup repository.
func NewBackupRepository(pool *pgxpool.Pool) *BackupRepository { return &BackupRepository{pool: pool} }

var _ backupdom.Repository = (*BackupRepository)(nil)

const backupColumns = `
	id, database_id, job_id, schedule_id, destination_id, type, engine, status, storage_url,
	size_bytes, checksum, started_at, completed_at, expires_at, error, created_by, created_at, version`

func (r *BackupRepository) Create(ctx context.Context, b *backupdom.Backup) error {
	const q = `
		INSERT INTO backups (id, database_id, job_id, schedule_id, destination_id, type, engine, status, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, version`
	err := r.pool.QueryRow(ctx, q,
		b.ID, b.DatabaseID, b.JobID, b.ScheduleID, b.DestinationID, b.Type, b.Engine, string(b.Status), b.ExpiresAt, b.CreatedBy,
	).Scan(&b.CreatedAt, &b.Version)
	if err != nil {
		return apperr.Internal(fmt.Errorf("insert backup: %w", err))
	}
	return nil
}

func (r *BackupRepository) GetByID(ctx context.Context, id uuid.UUID) (*backupdom.Backup, error) {
	q := `SELECT ` + backupColumns + ` FROM backups WHERE id = $1`
	b, err := scanBackup(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("backup not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get backup: %w", err))
	}
	return b, nil
}

func (r *BackupRepository) List(ctx context.Context, f backupdom.ListFilter) (backupdom.Page, error) {
	conds := []string{"true"}
	args := make([]any, 0, 4)
	if f.DatabaseID != nil {
		args = append(args, *f.DatabaseID)
		conds = append(conds, fmt.Sprintf("database_id = $%d", len(args)))
	}
	if f.Status != nil {
		args = append(args, string(*f.Status))
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.Scope != nil {
		args = append(args, idArray(f.Scope.DatabaseIDs))
		dbPos := len(args)
		args = append(args, idArray(f.Scope.ServerIDs))
		serverPos := len(args)
		conds = append(conds, fmt.Sprintf(
			"(database_id = ANY($%d) OR database_id IN (SELECT d.id FROM databases d JOIN instances i ON i.id = d.instance_id WHERE i.server_id = ANY($%d)))",
			dbPos, serverPos))
	}
	args = append(args, f.Limit)
	limitPos := len(args)
	args = append(args, f.Offset)
	offsetPos := len(args)

	q := fmt.Sprintf(
		`SELECT %s, count(*) OVER() AS total FROM backups WHERE %s
		 ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		backupColumns, join(conds), limitPos, offsetPos)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return backupdom.Page{}, apperr.Internal(fmt.Errorf("list backups: %w", err))
	}
	defer rows.Close()

	items := make([]*backupdom.Backup, 0)
	total := 0
	for rows.Next() {
		var (
			b      backupdom.Backup
			status string
		)
		if err := rows.Scan(
			&b.ID, &b.DatabaseID, &b.JobID, &b.ScheduleID, &b.DestinationID, &b.Type, &b.Engine, &status, &b.StorageURL,
			&b.SizeBytes, &b.Checksum, &b.StartedAt, &b.CompletedAt, &b.ExpiresAt, &b.Error, &b.CreatedBy, &b.CreatedAt, &b.Version,
			&total,
		); err != nil {
			return backupdom.Page{}, apperr.Internal(fmt.Errorf("scan backup: %w", err))
		}
		b.Status = backupdom.Status(status)
		items = append(items, &b)
	}
	if err := rows.Err(); err != nil {
		return backupdom.Page{}, apperr.Internal(err)
	}
	return backupdom.Page{Items: items, Total: total}, nil
}

func (r *BackupRepository) MarkRunning(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE backups SET status = 'running', started_at = now(), version = version + 1
		 WHERE id = $1 AND status = 'pending'`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("mark backup running: %w", err))
	}
	return nil
}

func (r *BackupRepository) Complete(ctx context.Context, id uuid.UUID, in backupdom.CompleteInput) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE backups SET status = $2, storage_url = COALESCE($3, storage_url),
			size_bytes = COALESCE($4, size_bytes), checksum = COALESCE($5, checksum),
			error = $6, completed_at = now(), version = version + 1
		WHERE id = $1 AND status IN ('pending','running')`,
		id, string(in.Status), in.StorageURL, in.SizeBytes, in.Checksum, in.Error)
	if err != nil {
		return apperr.Internal(fmt.Errorf("complete backup: %w", err))
	}
	return nil
}

func scanBackup(row rowScanner) (*backupdom.Backup, error) {
	var (
		b      backupdom.Backup
		status string
	)
	if err := row.Scan(
		&b.ID, &b.DatabaseID, &b.JobID, &b.ScheduleID, &b.DestinationID, &b.Type, &b.Engine, &status, &b.StorageURL,
		&b.SizeBytes, &b.Checksum, &b.StartedAt, &b.CompletedAt, &b.ExpiresAt, &b.Error, &b.CreatedBy, &b.CreatedAt, &b.Version,
	); err != nil {
		return nil, err
	}
	b.Status = backupdom.Status(status)
	return &b, nil
}

// ListExpired returns completed backups past their retention boundary.
func (r *BackupRepository) ListExpired(ctx context.Context, now time.Time, limit int) ([]backupdom.Expired, error) {
	const q = `
		SELECT id, destination_id, storage_url FROM backups
		WHERE status = 'completed' AND expires_at IS NOT NULL AND expires_at < $1
		  AND destination_id IS NOT NULL AND storage_url IS NOT NULL
		ORDER BY expires_at LIMIT $2`
	rows, err := r.pool.Query(ctx, q, now, limit)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list expired backups: %w", err))
	}
	defer rows.Close()
	out := make([]backupdom.Expired, 0)
	for rows.Next() {
		var e backupdom.Expired
		if err := rows.Scan(&e.ID, &e.DestinationID, &e.StorageURL); err != nil {
			return nil, apperr.Internal(fmt.Errorf("scan expired backup: %w", err))
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkExpired flags a backup as expired (its object has been deleted).
func (r *BackupRepository) MarkExpired(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE backups SET status = 'expired', version = version + 1 WHERE id = $1`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("mark backup expired: %w", err))
	}
	return nil
}

// CountByStatusSince counts backups grouped by status created since t.
func (r *BackupRepository) CountByStatusSince(ctx context.Context, since time.Time) (map[backupdom.Status]int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT status, count(*) FROM backups WHERE created_at >= $1 GROUP BY status`, since)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("count backups: %w", err))
	}
	defer rows.Close()
	out := make(map[backupdom.Status]int)
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, apperr.Internal(err)
		}
		out[backupdom.Status(status)] = n
	}
	return out, rows.Err()
}
