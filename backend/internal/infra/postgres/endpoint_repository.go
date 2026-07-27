package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	endpointdom "github.com/Fleetdock/fleetdock/backend/internal/domain/endpoint"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// EndpointRepository is the Postgres adapter for endpointdom.Repository.
type EndpointRepository struct {
	pool *pgxpool.Pool
}

// NewEndpointRepository builds an endpoint repository.
func NewEndpointRepository(pool *pgxpool.Pool) *EndpointRepository {
	return &EndpointRepository{pool: pool}
}

var _ endpointdom.Repository = (*EndpointRepository)(nil)

const endpointColumns = `
	id, database_id, access_type, status, protocol, external_host, external_port,
	internal_host, internal_port, tls_mode, tls_status, allowed_cidrs, max_connections,
	last_error, created_at, updated_at, disabled_at, version`

// portAllocationLock serializes port allocation across control-plane replicas.
// Any stable constant works; it only has to be unique among advisory locks.
const portAllocationLock = 8531104

// CreateWithPort allocates a free port and inserts the endpoint atomically.
func (r *EndpointRepository) CreateWithPort(ctx context.Context, e *endpointdom.Endpoint, start, end int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return apperr.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort on the non-commit path

	// Held until commit, so a concurrent enable cannot pick the same port
	// between the SELECT and the INSERT below.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, portAllocationLock); err != nil {
		return apperr.Internal(fmt.Errorf("lock port allocation: %w", err))
	}

	const allocate = `
		SELECT p FROM generate_series($1::int, $2::int) AS p
		WHERE p NOT IN (
			SELECT external_port FROM database_endpoints
			WHERE access_type = 'public'
			  AND status IN ('pending','active','disabling','error')
			  AND external_port IS NOT NULL
		)
		ORDER BY p LIMIT 1`
	var port int
	if err := tx.QueryRow(ctx, allocate, start, end).Scan(&port); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("no public ports available in the configured range")
		}
		return apperr.Internal(fmt.Errorf("allocate port: %w", err))
	}
	e.ExternalPort = &port

	const insert = `
		INSERT INTO database_endpoints (
			id, database_id, access_type, status, protocol, external_host, external_port,
			internal_host, internal_port, tls_mode, tls_status, allowed_cidrs, max_connections, last_error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING created_at, updated_at, version`
	err = tx.QueryRow(ctx, insert,
		e.ID, e.DatabaseID, string(e.AccessType), string(e.Status), string(e.Protocol),
		e.ExternalHost, e.ExternalPort, e.InternalHost, e.InternalPort,
		string(e.TLSMode), string(e.TLSStatus), e.AllowedCIDRs, e.MaxConnections, e.LastError,
	).Scan(&e.CreatedAt, &e.UpdatedAt, &e.Version)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("public access is already enabled for this database")
		}
		return apperr.Internal(fmt.Errorf("insert endpoint: %w", err))
	}
	return tx.Commit(ctx)
}

func (r *EndpointRepository) GetPublicByDatabaseID(ctx context.Context, databaseID uuid.UUID) (*endpointdom.Endpoint, error) {
	q := `SELECT ` + endpointColumns + `
		FROM database_endpoints
		WHERE database_id = $1 AND access_type = 'public' AND status <> 'disabled'
		ORDER BY created_at DESC LIMIT 1`
	e, err := scanEndpoint(r.pool.QueryRow(ctx, q, databaseID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("public endpoint not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get public endpoint: %w", err))
	}
	return e, nil
}

// ListRoutable returns endpoints that belong in the gateway config. Endpoints
// in error stay routed: the listener is programmed, the database behind it is
// just unhealthy, and removing it would free the port for reallocation.
func (r *EndpointRepository) ListRoutable(ctx context.Context) ([]*endpointdom.Endpoint, error) {
	return r.listPublicByStatus(ctx, []string{"pending", "active", "error"})
}

func (r *EndpointRepository) ListDisabling(ctx context.Context) ([]*endpointdom.Endpoint, error) {
	return r.listPublicByStatus(ctx, []string{"disabling"})
}

func (r *EndpointRepository) listPublicByStatus(ctx context.Context, statuses []string) ([]*endpointdom.Endpoint, error) {
	q := `SELECT ` + endpointColumns + `
		FROM database_endpoints
		WHERE access_type = 'public' AND status = ANY($1)
		ORDER BY external_port NULLS LAST, created_at`
	rows, err := r.pool.Query(ctx, q, statuses)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list public endpoints: %w", err))
	}
	defer rows.Close()
	return scanEndpointRows(rows)
}

func (r *EndpointRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status endpointdom.Status, lastError *string) error {
	// disabled_at is stamped once, on the transition into disabled, and left
	// alone otherwise so the original timestamp survives later updates.
	const q = `
		UPDATE database_endpoints
		SET status = $2,
		    last_error = $3,
		    disabled_at = CASE WHEN $2 = 'disabled' THEN now() ELSE disabled_at END
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, string(status), lastError)
	if err != nil {
		return apperr.Internal(fmt.Errorf("update endpoint status: %w", err))
	}
	return nil
}

func (r *EndpointRepository) UpdateBackend(ctx context.Context, id uuid.UUID, internalHost string, internalPort int) error {
	const q = `
		UPDATE database_endpoints
		SET internal_host = $2, internal_port = $3
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, internalHost, internalPort)
	if err != nil {
		return apperr.Internal(fmt.Errorf("update endpoint backend: %w", err))
	}
	return nil
}

func (r *EndpointRepository) UpdateTLSStatus(ctx context.Context, id uuid.UUID, tlsStatus endpointdom.TLSStatus) error {
	const q = `UPDATE database_endpoints SET tls_status = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, string(tlsStatus))
	if err != nil {
		return apperr.Internal(fmt.Errorf("update endpoint tls status: %w", err))
	}
	return nil
}

func (r *EndpointRepository) UpdateAllowedCIDRs(ctx context.Context, id uuid.UUID, cidrs []string) error {
	const q = `UPDATE database_endpoints SET allowed_cidrs = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, cidrs)
	if err != nil {
		return apperr.Internal(fmt.Errorf("update endpoint cidrs: %w", err))
	}
	return nil
}

func (r *EndpointRepository) TransferDatabase(ctx context.Context, fromDatabaseID, toDatabaseID uuid.UUID) error {
	const q = `
		UPDATE database_endpoints SET database_id = $2
		WHERE database_id = $1 AND access_type = 'public' AND status <> 'disabled'`
	_, err := r.pool.Exec(ctx, q, fromDatabaseID, toDatabaseID)
	if err != nil {
		return apperr.Internal(fmt.Errorf("transfer endpoint: %w", err))
	}
	return nil
}

func (r *EndpointRepository) DisablePublic(ctx context.Context, databaseID uuid.UUID) error {
	const q = `
		UPDATE database_endpoints
		SET status = 'disabled', disabled_at = now()
		WHERE database_id = $1 AND access_type = 'public' AND status <> 'disabled'`
	_, err := r.pool.Exec(ctx, q, databaseID)
	if err != nil {
		return apperr.Internal(fmt.Errorf("disable public endpoint: %w", err))
	}
	return nil
}

func scanEndpoint(row pgx.Row) (*endpointdom.Endpoint, error) {
	var e endpointdom.Endpoint
	var accessType, status, protocol, tlsMode, tlsStatus string
	err := row.Scan(
		&e.ID, &e.DatabaseID, &accessType, &status, &protocol, &e.ExternalHost, &e.ExternalPort,
		&e.InternalHost, &e.InternalPort, &tlsMode, &tlsStatus, &e.AllowedCIDRs, &e.MaxConnections,
		&e.LastError, &e.CreatedAt, &e.UpdatedAt, &e.DisabledAt, &e.Version,
	)
	if err != nil {
		return nil, err
	}
	e.AccessType = endpointdom.AccessType(accessType)
	e.Status = endpointdom.Status(status)
	e.Protocol = endpointdom.Protocol(protocol)
	e.TLSMode = endpointdom.TLSMode(tlsMode)
	e.TLSStatus = endpointdom.TLSStatus(tlsStatus)
	return &e, nil
}

func scanEndpointRows(rows pgx.Rows) ([]*endpointdom.Endpoint, error) {
	out := make([]*endpointdom.Endpoint, 0)
	for rows.Next() {
		e, err := scanEndpoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
