package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authzapp "github.com/TajBrains/fleetdock/backend/internal/app/authz"
	"github.com/TajBrains/fleetdock/backend/internal/platform/apperr"
)

// AuthzRepository resolves resource lineage for scoped authorization.
type AuthzRepository struct {
	pool *pgxpool.Pool
}

// NewAuthzRepository builds the resolver adapter.
func NewAuthzRepository(pool *pgxpool.Pool) *AuthzRepository { return &AuthzRepository{pool: pool} }

var _ authzapp.Repository = (*AuthzRepository)(nil)

// ServerOfInstance returns the server id owning an instance.
func (r *AuthzRepository) ServerOfInstance(ctx context.Context, instanceID uuid.UUID) (uuid.UUID, error) {
	var sid uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT server_id FROM instances WHERE id = $1`, instanceID).Scan(&sid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, apperr.NotFound("instance not found")
		}
		return uuid.Nil, apperr.Internal(fmt.Errorf("server of instance: %w", err))
	}
	return sid, nil
}

// LineageOfDatabase returns the instance and server ids owning a database.
func (r *AuthzRepository) LineageOfDatabase(ctx context.Context, databaseID uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	var iid, sid uuid.UUID
	const q = `
		SELECT d.instance_id, i.server_id
		FROM databases d
		JOIN instances i ON i.id = d.instance_id
		WHERE d.id = $1`
	err := r.pool.QueryRow(ctx, q, databaseID).Scan(&iid, &sid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, uuid.Nil, apperr.NotFound("database not found")
		}
		return uuid.Nil, uuid.Nil, apperr.Internal(fmt.Errorf("lineage of database: %w", err))
	}
	return iid, sid, nil
}

// DatabaseOfBackup returns the database id a backup belongs to.
func (r *AuthzRepository) DatabaseOfBackup(ctx context.Context, backupID uuid.UUID) (uuid.UUID, error) {
	var dbID uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT database_id FROM backups WHERE id = $1`, backupID).Scan(&dbID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, apperr.NotFound("backup not found")
		}
		return uuid.Nil, apperr.Internal(fmt.Errorf("database of backup: %w", err))
	}
	return dbID, nil
}

// JobResource returns the resource_type and resource_id of a job.
func (r *AuthzRepository) JobResource(ctx context.Context, jobID uuid.UUID) (string, uuid.UUID, error) {
	var (
		resType string
		resID   *uuid.UUID
	)
	err := r.pool.QueryRow(ctx, `SELECT resource_type, resource_id FROM jobs WHERE id = $1`, jobID).Scan(&resType, &resID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", uuid.Nil, apperr.NotFound("operation not found")
		}
		return "", uuid.Nil, apperr.Internal(fmt.Errorf("job resource: %w", err))
	}
	if resID == nil {
		return resType, uuid.Nil, nil
	}
	return resType, *resID, nil
}
