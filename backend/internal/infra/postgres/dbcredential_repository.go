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

	dbcredentialdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbcredential"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// DBCredentialRepository is the Postgres adapter for dbcredentialdom.Repository.
type DBCredentialRepository struct {
	pool *pgxpool.Pool
}

// NewDBCredentialRepository builds a credential repository.
func NewDBCredentialRepository(pool *pgxpool.Pool) *DBCredentialRepository {
	return &DBCredentialRepository{pool: pool}
}

var _ dbcredentialdom.Repository = (*DBCredentialRepository)(nil)

const credentialColumns = `
	id, database_id, name, username, secret_ref, access_level, account_host,
	expires_at, revoked_at, created_at, updated_at, version`

func (r *DBCredentialRepository) Create(ctx context.Context, c *dbcredentialdom.Credential) error {
	const q = `
		INSERT INTO database_credentials (
			id, database_id, name, username, secret_ref, access_level, account_host, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING created_at, updated_at, version`
	err := r.pool.QueryRow(ctx, q,
		c.ID, c.DatabaseID, c.Name, c.Username, c.SecretRef, string(c.AccessLevel), c.AccountHost, c.ExpiresAt,
	).Scan(&c.CreatedAt, &c.UpdatedAt, &c.Version)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a credential with this name or username already exists")
		}
		return apperr.Internal(fmt.Errorf("insert credential: %w", err))
	}
	return nil
}

func (r *DBCredentialRepository) GetByID(ctx context.Context, id uuid.UUID) (*dbcredentialdom.Credential, error) {
	q := `SELECT ` + credentialColumns + ` FROM database_credentials WHERE id = $1`
	c, err := scanCredential(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("credential not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get credential: %w", err))
	}
	return c, nil
}

func (r *DBCredentialRepository) ListByDatabaseID(ctx context.Context, databaseID uuid.UUID) ([]*dbcredentialdom.Credential, error) {
	q := `SELECT ` + credentialColumns + `
		FROM database_credentials WHERE database_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, databaseID)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list credentials: %w", err))
	}
	defer rows.Close()
	out := make([]*dbcredentialdom.Credential, 0)
	for rows.Next() {
		c, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *DBCredentialRepository) Revoke(ctx context.Context, id uuid.UUID, at time.Time) error {
	const q = `
		UPDATE database_credentials SET revoked_at = $2, version = version + 1
		WHERE id = $1 AND revoked_at IS NULL`
	tag, err := r.pool.Exec(ctx, q, id, at)
	if err != nil {
		return apperr.Internal(fmt.Errorf("revoke credential: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("credential not found or already revoked")
	}
	return nil
}

func (r *DBCredentialRepository) ListExpired(ctx context.Context, now time.Time) ([]*dbcredentialdom.Credential, error) {
	q := `SELECT ` + credentialColumns + `
		FROM database_credentials
		WHERE revoked_at IS NULL AND expires_at IS NOT NULL AND expires_at <= $1`
	rows, err := r.pool.Query(ctx, q, now)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list expired credentials: %w", err))
	}
	defer rows.Close()
	out := make([]*dbcredentialdom.Credential, 0)
	for rows.Next() {
		c, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanCredential(row pgx.Row) (*dbcredentialdom.Credential, error) {
	var c dbcredentialdom.Credential
	var accessLevel string
	err := row.Scan(
		&c.ID, &c.DatabaseID, &c.Name, &c.Username, &c.SecretRef, &accessLevel, &c.AccountHost,
		&c.ExpiresAt, &c.RevokedAt, &c.CreatedAt, &c.UpdatedAt, &c.Version,
	)
	if err != nil {
		return nil, err
	}
	c.AccessLevel = dbcredentialdom.AccessLevel(accessLevel)
	return &c, nil
}
