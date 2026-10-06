package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
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
//
// databaseColumns is unqualified, for UPDATE ... RETURNING. databaseColumnsD is
// the same list qualified with the `d` alias, for the SELECTs that join
// instances. The two must stay in the same order — scanDatabase reads both.
const databaseColumns = `
	id, instance_id, name, charset, "collation", status, system, size_bytes, active_connections,
	locked_at, locked_by, labels, tags, created_at, updated_at, version, deleted_at, missing_since`

const databaseColumnsD = `
	d.id, d.instance_id, d.name, d.charset, d."collation", d.status, d.system, d.size_bytes, d.active_connections,
	d.locked_at, d.locked_by, d.labels, d.tags, d.created_at, d.updated_at, d.version, d.deleted_at, d.missing_since`

// instanceRefColumns is the owning-instance summary joined onto reads.
const instanceRefColumns = `i.id, i.name, i.engine, i.kind, i.server_id, i.container_id, i.username, i.root_secret_ref`

func (r *DatabaseRepository) Create(ctx context.Context, d *databasedom.Database) error {
	labels, err := json.Marshal(d.Labels)
	if err != nil {
		return apperr.Internal(fmt.Errorf("marshal labels: %w", err))
	}
	const q = `
		INSERT INTO databases (id, instance_id, name, charset, "collation", status, system, labels, tags)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)
		RETURNING created_at, updated_at, version`
	err = r.pool.QueryRow(ctx, q,
		d.ID, d.InstanceID, d.Name, d.Charset, d.Collation, string(d.Status), d.System, string(labels), d.Tags,
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
	q := `SELECT ` + databaseColumnsD + `, ` + instanceRefColumns + `
		FROM databases d JOIN instances i ON i.id = d.instance_id AND i.deleted_at IS NULL
		WHERE d.id = $1 AND d.deleted_at IS NULL`
	d, err := scanDatabaseWithInstance(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("database not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get database: %w", err))
	}
	return d, nil
}

func (r *DatabaseRepository) List(ctx context.Context, f databasedom.ListFilter) (databasedom.Page, error) {
	// Every condition is qualified with the `d` alias: joining instances makes
	// bare id/name/status/deleted_at ambiguous.
	conds := []string{"d.deleted_at IS NULL"}
	args := make([]any, 0, 5)
	if f.InstanceID != nil {
		args = append(args, *f.InstanceID)
		conds = append(conds, fmt.Sprintf("d.instance_id = $%d", len(args)))
	}
	if f.Status != nil {
		args = append(args, string(*f.Status))
		conds = append(conds, fmt.Sprintf("d.status = $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+likeEscape(f.Search)+"%")
		conds = append(conds, fmt.Sprintf("d.name ILIKE $%d", len(args)))
	}
	if f.Scope != nil {
		args = append(args, idArray(f.Scope.DatabaseIDs))
		dbPos := len(args)
		args = append(args, idArray(f.Scope.ServerIDs))
		serverPos := len(args)
		conds = append(conds, fmt.Sprintf(
			"(d.id = ANY($%d) OR i.server_id = ANY($%d))",
			dbPos, serverPos))
	}
	args = append(args, f.Limit)
	limitPos := len(args)
	args = append(args, f.Offset)
	offsetPos := len(args)

	q := fmt.Sprintf(
		`SELECT %s, %s, count(*) OVER() AS total
		 FROM databases d JOIN instances i ON i.id = d.instance_id AND i.deleted_at IS NULL
		 WHERE %s
		 ORDER BY d.created_at DESC
		 LIMIT $%d OFFSET $%d`,
		databaseColumnsD, instanceRefColumns, join(conds), limitPos, offsetPos,
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
		&d.ID, &d.InstanceID, &d.Name, &d.Charset, &d.Collation, &status, &d.System, &d.SizeBytes, &d.ActiveConnections,
		&d.LockedAt, &d.LockedBy, &labelsRaw, &d.Tags, &d.CreatedAt, &d.UpdatedAt, &d.Version, &d.DeletedAt, &d.MissingSince,
	); err != nil {
		return nil, err
	}
	return finishDatabase(&d, labelsRaw, status)
}

func scanDatabaseWithInstance(row rowScanner) (*databasedom.Database, error) {
	var (
		d         databasedom.Database
		labelsRaw []byte
		status    string
		ref       instanceRefScan
	)
	if err := row.Scan(
		&d.ID, &d.InstanceID, &d.Name, &d.Charset, &d.Collation, &status, &d.System, &d.SizeBytes, &d.ActiveConnections,
		&d.LockedAt, &d.LockedBy, &labelsRaw, &d.Tags, &d.CreatedAt, &d.UpdatedAt, &d.Version, &d.DeletedAt, &d.MissingSince,
		&ref.id, &ref.name, &ref.engine, &ref.kind, &ref.serverID, &ref.containerID, &ref.username, &ref.rootSecretRef,
	); err != nil {
		return nil, err
	}
	d.Instance = ref.build()
	return finishDatabase(&d, labelsRaw, status)
}

func scanDatabaseWithTotal(row rowScanner) (*databasedom.Database, int, error) {
	var (
		d         databasedom.Database
		labelsRaw []byte
		status    string
		ref       instanceRefScan
		total     int
	)
	if err := row.Scan(
		&d.ID, &d.InstanceID, &d.Name, &d.Charset, &d.Collation, &status, &d.System, &d.SizeBytes, &d.ActiveConnections,
		&d.LockedAt, &d.LockedBy, &labelsRaw, &d.Tags, &d.CreatedAt, &d.UpdatedAt, &d.Version, &d.DeletedAt, &d.MissingSince,
		&ref.id, &ref.name, &ref.engine, &ref.kind, &ref.serverID, &ref.containerID, &ref.username, &ref.rootSecretRef, &total,
	); err != nil {
		return nil, 0, err
	}
	d.Instance = ref.build()
	out, err := finishDatabase(&d, labelsRaw, status)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// instanceRefScan holds the joined instance columns before they are folded into
// a domain InstanceRef.
type instanceRefScan struct {
	id            uuid.UUID
	name          string
	engine        string
	kind          string
	serverID      *uuid.UUID
	containerID   *string
	username      *string
	rootSecretRef *string
}

// build folds the scanned columns into the domain ref. The two derived booleans
// intentionally restate instance.Provisioned() and instance.HasCredentials();
// keep them in sync with internal/domain/instance/instance.go.
func (s instanceRefScan) build() *databasedom.InstanceRef {
	return &databasedom.InstanceRef{
		ID:             s.id,
		Name:           s.name,
		Engine:         s.engine,
		Kind:           s.kind,
		ServerID:       s.serverID,
		Provisioned:    s.kind == "managed" && s.containerID != nil && *s.containerID != "",
		HasCredentials: s.username != nil && *s.username != "" && s.rootSecretRef != nil,
	}
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

// Reconcile brings an instance's database rows in line with what a probe saw
// on the server, in one transaction:
//   - databases seen for the first time are added (unless the user removed
//     them from Fleetdock within the recovery window);
//   - seen databases get fresh size/connection counts and last_seen_at, and
//     a "missing" one becomes active again;
//   - active databases unseen for longer than MissingAfter are marked
//     missing. Nothing is ever deleted, and databases being created, moved or
//     deleted are left alone.
func (r *DatabaseRepository) Reconcile(ctx context.Context, instanceID uuid.UUID, seen []databasedom.Observed, now time.Time) (databasedom.ReconcileResult, error) {
	var res databasedom.ReconcileResult
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return res, apperr.Internal(fmt.Errorf("begin reconcile: %w", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	names := make([]string, len(seen))
	for i, o := range seen {
		names[i] = o.Name
	}

	for _, o := range seen {
		var id uuid.UUID
		var status string
		err := tx.QueryRow(ctx, `
			SELECT id, status FROM databases
			WHERE instance_id = $1 AND name = $2 AND deleted_at IS NULL`, instanceID, o.Name).Scan(&id, &status)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			var recentlyRemoved bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (SELECT 1 FROM databases
				  WHERE instance_id = $1 AND name = $2 AND deleted_at > $3)`,
				instanceID, o.Name, now.Add(-databasedom.RediscoverAfter)).Scan(&recentlyRemoved); err != nil {
				return res, apperr.Internal(fmt.Errorf("check removed database: %w", err))
			}
			if recentlyRemoved {
				continue
			}
			d, err := databasedom.NewDatabase(instanceID, o.Name, o.Charset, o.Collation, map[string]string{"discovered": "true"}, nil)
			if err != nil {
				continue // a name Fleetdock cannot manage; skip it
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO databases (id, instance_id, name, charset, "collation", status, system, labels, tags,
				                       size_bytes, active_connections, last_seen_at)
				VALUES ($1, $2, $3, $4, $5, 'active', $6, '{"discovered":"true"}'::jsonb, '{}', $7, $8, $9)
				ON CONFLICT DO NOTHING`,
				d.ID, instanceID, d.Name, d.Charset, d.Collation, o.System, o.SizeBytes, o.Connections, now); err != nil {
				return res, apperr.Internal(fmt.Errorf("add discovered database: %w", err))
			}
			res.Added = append(res.Added, o.Name)
		case err != nil:
			return res, apperr.Internal(fmt.Errorf("look up database: %w", err))
		default:
			if _, err := tx.Exec(ctx, `
				UPDATE databases SET size_bytes = $2, active_connections = $3, last_seen_at = $4,
				  status = CASE WHEN status = 'missing' THEN 'active' ELSE status END,
				  missing_since = CASE WHEN status = 'missing' THEN NULL ELSE missing_since END
				WHERE id = $1`, id, o.SizeBytes, o.Connections, now); err != nil {
				return res, apperr.Internal(fmt.Errorf("update database stats: %w", err))
			}
			if status == string(databasedom.StatusMissing) {
				res.Reappeared = append(res.Reappeared, o.Name)
			}
		}
	}

	rows, err := tx.Query(ctx, `
		UPDATE databases SET status = 'missing', missing_since = $3, version = version + 1
		WHERE instance_id = $1 AND deleted_at IS NULL AND status = 'active'
		  AND NOT (name = ANY($2::text[]))
		  AND COALESCE(last_seen_at, created_at) < $4
		RETURNING name`, instanceID, names, now, now.Add(-databasedom.MissingAfter))
	if err != nil {
		return res, apperr.Internal(fmt.Errorf("mark missing databases: %w", err))
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return res, apperr.Internal(err)
		}
		res.Missing = append(res.Missing, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, apperr.Internal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return res, apperr.Internal(fmt.Errorf("commit reconcile: %w", err))
	}
	return res, nil
}
