package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	backupdom "github.com/mariadb-cp/db-manager/backend/internal/domain/backup"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
)

// BackupRepository is the Postgres adapter for backupdom.Repository.
type BackupRepository struct {
	pool *pgxpool.Pool
}

// NewBackupRepository builds a backup repository.
func NewBackupRepository(pool *pgxpool.Pool) *BackupRepository { return &BackupRepository{pool: pool} }

var _ backupdom.Repository = (*BackupRepository)(nil)

const backupColumns = `
	id, database_id, job_id, destination_id, type, engine, status, storage_url,
	size_bytes, checksum, started_at, completed_at, error, created_by, created_at, version`

func (r *BackupRepository) Create(ctx context.Context, b *backupdom.Backup) error {
	const q = `
		INSERT INTO backups (id, database_id, job_id, destination_id, type, engine, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, version`
	err := r.pool.QueryRow(ctx, q,
		b.ID, b.DatabaseID, b.JobID, b.DestinationID, b.Type, b.Engine, string(b.Status), b.CreatedBy,
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
			&b.ID, &b.DatabaseID, &b.JobID, &b.DestinationID, &b.Type, &b.Engine, &status, &b.StorageURL,
			&b.SizeBytes, &b.Checksum, &b.StartedAt, &b.CompletedAt, &b.Error, &b.CreatedBy, &b.CreatedAt, &b.Version,
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
		&b.ID, &b.DatabaseID, &b.JobID, &b.DestinationID, &b.Type, &b.Engine, &status, &b.StorageURL,
		&b.SizeBytes, &b.Checksum, &b.StartedAt, &b.CompletedAt, &b.Error, &b.CreatedBy, &b.CreatedAt, &b.Version,
	); err != nil {
		return nil, err
	}
	b.Status = backupdom.Status(status)
	return &b, nil
}
