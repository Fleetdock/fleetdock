package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	databasedom "github.com/TajBrains/db-manager/backend/internal/domain/database"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// DatabaseRepository is the Postgres adapter for databasedom.Repository.
type DatabaseRepository struct {
	pool *pgxpool.Pool
}

// NewDatabaseRepository builds a database repository.
func NewDatabaseRepository(pool *pgxpool.Pool) *DatabaseRepository {
	return &DatabaseRepository{pool: pool}
}

var _ databasedom.Repository = (*DatabaseRepository)(nil)

// Note: "collation" is quoted because it is a reserved word in PostgreSQL.
const databaseColumns = `
	id, instance_id, name, charset, "collation", status, size_bytes, active_connections,
	locked_at, locked_by, labels, tags, created_at, updated_at, version, deleted_at`

func (r *DatabaseRepository) Create(ctx context.Context, d *databasedom.Database) error {
	labels, err := json.Marshal(d.Labels)
	if err != nil {
		return apperr.Internal(fmt.Errorf("marshal labels: %w", err))
	}
	const q = `
		INSERT INTO databases (id, instance_id, name, charset, "collation", status, labels, tags)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
		RETURNING created_at, updated_at, version`
	err = r.pool.QueryRow(ctx, q,
		d.ID, d.InstanceID, d.Name, d.Charset, d.Collation, string(d.Status), string(labels), d.Tags,
	).Scan(&d.CreatedAt, &d.UpdatedAt, &d.Version)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case uniqueViolation:
				return apperr.Conflict("a database with this name already exists on the instance")
			case foreignKeyViolation:
				return apperr.Invalid("instance_id", "instance does not exist")
			}
		}
		return apperr.Internal(fmt.Errorf("insert database: %w", err))
	}
	return nil
}

func (r *DatabaseRepository) GetByID(ctx context.Context, id uuid.UUID) (*databasedom.Database, error) {
	q := `SELECT ` + databaseColumns + ` FROM databases WHERE id = $1 AND deleted_at IS NULL`
	d, err := scanDatabase(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("database not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get database: %w", err))
	}
	return d, nil
}

func (r *DatabaseRepository) List(ctx context.Context, f databasedom.ListFilter) (databasedom.Page, error) {
	conds := []string{"deleted_at IS NULL"}
	args := make([]any, 0, 5)
	if f.InstanceID != nil {
		args = append(args, *f.InstanceID)
		conds = append(conds, fmt.Sprintf("instance_id = $%d", len(args)))
	}
	if f.Status != nil {
		args = append(args, string(*f.Status))
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		conds = append(conds, fmt.Sprintf("name ILIKE $%d", len(args)))
	}
	args = append(args, f.Limit)
	limitPos := len(args)
	args = append(args, f.Offset)
	offsetPos := len(args)

	q := fmt.Sprintf(
		`SELECT %s, count(*) OVER() AS total
		 FROM databases WHERE %s
		 ORDER BY created_at DESC
		 LIMIT $%d OFFSET $%d`,
		databaseColumns, join(conds), limitPos, offsetPos,
	)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return databasedom.Page{}, apperr.Internal(fmt.Errorf("list databases: %w", err))
	}
	defer rows.Close()

	items := make([]*databasedom.Database, 0)
	total := 0
	for rows.Next() {
		d, t, err := scanDatabaseWithTotal(rows)
		if err != nil {
			return databasedom.Page{}, apperr.Internal(fmt.Errorf("scan database: %w", err))
		}
		items = append(items, d)
		total = t
	}
	if err := rows.Err(); err != nil {
		return databasedom.Page{}, apperr.Internal(err)
	}
	return databasedom.Page{Items: items, Total: total}, nil
}

func (r *DatabaseRepository) Lock(ctx context.Context, id, lockedBy uuid.UUID) (*databasedom.Database, error) {
	q := `
		UPDATE databases
		SET status = 'locked', locked_at = now(), locked_by = $2, version = version + 1
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING ` + databaseColumns
	d, err := scanDatabase(r.pool.QueryRow(ctx, q, id, lockedBy))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("database not found")
		}
		return nil, apperr.Internal(fmt.Errorf("lock database: %w", err))
	}
	return d, nil
}

func (r *DatabaseRepository) Unlock(ctx context.Context, id uuid.UUID) (*databasedom.Database, error) {
	q := `
		UPDATE databases
		SET status = 'active', locked_at = NULL, locked_by = NULL, version = version + 1
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING ` + databaseColumns
	d, err := scanDatabase(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("database not found")
		}
		return nil, apperr.Internal(fmt.Errorf("unlock database: %w", err))
	}
	return d, nil
}

func (r *DatabaseRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE databases
		SET status = 'deleting', deleted_at = now(),
		    purge_after = now() + interval '7 days', version = version + 1
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("delete database: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("database not found")
	}
	return nil
}

func scanDatabase(row rowScanner) (*databasedom.Database, error) {
	var (
		d         databasedom.Database
		labelsRaw []byte
		status    string
	)
	if err := row.Scan(
		&d.ID, &d.InstanceID, &d.Name, &d.Charset, &d.Collation, &status, &d.SizeBytes, &d.ActiveConnections,
		&d.LockedAt, &d.LockedBy, &labelsRaw, &d.Tags, &d.CreatedAt, &d.UpdatedAt, &d.Version, &d.DeletedAt,
	); err != nil {
		return nil, err
	}
	return finishDatabase(&d, labelsRaw, status)
}

func scanDatabaseWithTotal(row rowScanner) (*databasedom.Database, int, error) {
	var (
		d         databasedom.Database
		labelsRaw []byte
		status    string
		total     int
	)
	if err := row.Scan(
		&d.ID, &d.InstanceID, &d.Name, &d.Charset, &d.Collation, &status, &d.SizeBytes, &d.ActiveConnections,
		&d.LockedAt, &d.LockedBy, &labelsRaw, &d.Tags, &d.CreatedAt, &d.UpdatedAt, &d.Version, &d.DeletedAt, &total,
	); err != nil {
		return nil, 0, err
	}
	out, err := finishDatabase(&d, labelsRaw, status)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func finishDatabase(d *databasedom.Database, labelsRaw []byte, status string) (*databasedom.Database, error) {
	d.Status = databasedom.Status(status)
	if len(labelsRaw) > 0 {
		if err := json.Unmarshal(labelsRaw, &d.Labels); err != nil {
			return nil, err
		}
	}
	if d.Labels == nil {
		d.Labels = map[string]string{}
	}
	if d.Tags == nil {
		d.Tags = []string{}
	}
	return d, nil
}
