package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	backupdestdom "github.com/TajBrains/fleetdock/backend/internal/domain/backupdest"
	"github.com/TajBrains/fleetdock/backend/internal/platform/apperr"
)

// BackupDestRepository is the Postgres adapter for backupdestdom.Repository.
type BackupDestRepository struct {
	pool *pgxpool.Pool
}

// NewBackupDestRepository builds a backup-destination repository.
func NewBackupDestRepository(pool *pgxpool.Pool) *BackupDestRepository {
	return &BackupDestRepository{pool: pool}
}

var _ backupdestdom.Repository = (*BackupDestRepository)(nil)

const destColumns = `
	id, name, provider, bucket, region, endpoint, prefix, access_key_id, secret_ref,
	created_by, created_at, updated_at, version, deleted_at`

func (r *BackupDestRepository) Create(ctx context.Context, d *backupdestdom.Destination) error {
	const q = `
		INSERT INTO backup_destinations (id, name, provider, bucket, region, endpoint, prefix, access_key_id, secret_ref, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at, version`
	err := r.pool.QueryRow(ctx, q,
		d.ID, d.Name, string(d.Provider), d.Bucket, d.Region, d.Endpoint, d.Prefix,
		d.AccessKeyID, d.SecretRef, d.CreatedBy,
	).Scan(&d.CreatedAt, &d.UpdatedAt, &d.Version)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a destination with this name already exists")
		}
		return apperr.Internal(fmt.Errorf("insert destination: %w", err))
	}
	return nil
}

func (r *BackupDestRepository) GetByID(ctx context.Context, id uuid.UUID) (*backupdestdom.Destination, error) {
	q := `SELECT ` + destColumns + ` FROM backup_destinations WHERE id = $1 AND deleted_at IS NULL`
	d, err := scanDest(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("backup destination not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get destination: %w", err))
	}
	return d, nil
}

func (r *BackupDestRepository) List(ctx context.Context) ([]*backupdestdom.Destination, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+destColumns+` FROM backup_destinations WHERE deleted_at IS NULL ORDER BY created_at DESC`)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list destinations: %w", err))
	}
	defer rows.Close()
	out := make([]*backupdestdom.Destination, 0)
	for rows.Next() {
		d, err := scanDest(rows)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *BackupDestRepository) Update(ctx context.Context, d *backupdestdom.Destination) error {
	const q = `
		UPDATE backup_destinations SET
			name = $2, provider = $3, bucket = $4, region = $5, endpoint = $6,
			prefix = $7, access_key_id = $8, version = version + 1
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING updated_at, version`
	err := r.pool.QueryRow(ctx, q,
		d.ID, d.Name, string(d.Provider), d.Bucket, d.Region, d.Endpoint, d.Prefix, d.AccessKeyID,
	).Scan(&d.UpdatedAt, &d.Version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("backup destination not found")
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a destination with this name already exists")
		}
		return apperr.Internal(fmt.Errorf("update destination: %w", err))
	}
	return nil
}

func (r *BackupDestRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE backup_destinations SET deleted_at = now(), version = version + 1
		 WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("delete destination: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("backup destination not found")
	}
	return nil
}

func scanDest(row rowScanner) (*backupdestdom.Destination, error) {
	var (
		d        backupdestdom.Destination
		provider string
	)
	if err := row.Scan(
		&d.ID, &d.Name, &provider, &d.Bucket, &d.Region, &d.Endpoint, &d.Prefix,
		&d.AccessKeyID, &d.SecretRef, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.Version, &d.DeletedAt,
	); err != nil {
		return nil, err
	}
	d.Provider = backupdestdom.Provider(provider)
	return &d, nil
}
