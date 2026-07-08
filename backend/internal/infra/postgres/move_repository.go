package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	movedom "github.com/mariadb-cp/db-manager/backend/internal/domain/move"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
)

// MoveRepository is the Postgres adapter for movedom.Repository.
type MoveRepository struct {
	pool *pgxpool.Pool
}

// NewMoveRepository builds a move repository.
func NewMoveRepository(pool *pgxpool.Pool) *MoveRepository { return &MoveRepository{pool: pool} }

var _ movedom.Repository = (*MoveRepository)(nil)

const moveColumns = `
	id, source_database_id, target_instance_id, target_database, destination_id,
	drop_source, backup_id, restore_job_id, status, table_count, error,
	created_by, created_at, updated_at, version`

func (r *MoveRepository) Create(ctx context.Context, m *movedom.Move) error {
	const q = `
		INSERT INTO db_moves
			(id, source_database_id, target_instance_id, target_database, destination_id, drop_source, backup_id, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at, version`
	err := r.pool.QueryRow(ctx, q,
		m.ID, m.SourceDatabaseID, m.TargetInstanceID, m.TargetDatabase, m.DestinationID,
		m.DropSource, m.BackupID, string(m.Status), m.CreatedBy,
	).Scan(&m.CreatedAt, &m.UpdatedAt, &m.Version)
	if err != nil {
		return apperr.Internal(fmt.Errorf("insert move: %w", err))
	}
	return nil
}

func (r *MoveRepository) GetByID(ctx context.Context, id uuid.UUID) (*movedom.Move, error) {
	return r.getOne(ctx, `WHERE id = $1`, id)
}

func (r *MoveRepository) GetByBackupID(ctx context.Context, backupID uuid.UUID) (*movedom.Move, error) {
	return r.getOne(ctx, `WHERE backup_id = $1`, backupID)
}

func (r *MoveRepository) GetByRestoreJobID(ctx context.Context, jobID uuid.UUID) (*movedom.Move, error) {
	return r.getOne(ctx, `WHERE restore_job_id = $1`, jobID)
}

func (r *MoveRepository) getOne(ctx context.Context, where string, arg any) (*movedom.Move, error) {
	q := `SELECT ` + moveColumns + ` FROM db_moves ` + where + ` ORDER BY created_at DESC LIMIT 1`
	m, err := scanMove(r.pool.QueryRow(ctx, q, arg))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("move not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get move: %w", err))
	}
	return m, nil
}

func (r *MoveRepository) List(ctx context.Context) ([]*movedom.Move, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+moveColumns+` FROM db_moves ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list moves: %w", err))
	}
	defer rows.Close()
	out := make([]*movedom.Move, 0)
	for rows.Next() {
		m, err := scanMove(rows)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *MoveRepository) Update(ctx context.Context, m *movedom.Move) error {
	const q = `
		UPDATE db_moves SET
			status = $2, backup_id = $3, restore_job_id = $4, table_count = $5, error = $6,
			version = version + 1
		WHERE id = $1
		RETURNING updated_at, version`
	err := r.pool.QueryRow(ctx, q,
		m.ID, string(m.Status), m.BackupID, m.RestoreJobID, m.TableCount, m.Error,
	).Scan(&m.UpdatedAt, &m.Version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("move not found")
		}
		return apperr.Internal(fmt.Errorf("update move: %w", err))
	}
	return nil
}

func scanMove(row rowScanner) (*movedom.Move, error) {
	var (
		m      movedom.Move
		status string
	)
	if err := row.Scan(
		&m.ID, &m.SourceDatabaseID, &m.TargetInstanceID, &m.TargetDatabase, &m.DestinationID,
		&m.DropSource, &m.BackupID, &m.RestoreJobID, &status, &m.TableCount, &m.Error,
		&m.CreatedBy, &m.CreatedAt, &m.UpdatedAt, &m.Version,
	); err != nil {
		return nil, err
	}
	m.Status = movedom.Status(status)
	return &m, nil
}
