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

	instancedom "github.com/TajBrains/fleetdock/backend/internal/domain/instance"
	"github.com/TajBrains/fleetdock/backend/internal/platform/apperr"
)

const foreignKeyViolation = "23503"

// InstanceRepository is the Postgres adapter for instancedom.Repository.
type InstanceRepository struct {
	pool *pgxpool.Pool
}

// NewInstanceRepository builds an instance repository.
func NewInstanceRepository(pool *pgxpool.Pool) *InstanceRepository {
	return &InstanceRepository{pool: pool}
}

var _ instancedom.Repository = (*InstanceRepository)(nil)

const instanceColumns = `
	id, server_id, name, engine, kind, host, username, root_secret_ref,
	container_id, mariadb_version, port, status,
	labels, tags, created_at, updated_at, version, deleted_at`

func (r *InstanceRepository) Create(ctx context.Context, in *instancedom.Instance) error {
	labels, err := json.Marshal(in.Labels)
	if err != nil {
		return apperr.Internal(fmt.Errorf("marshal labels: %w", err))
	}
	const q = `
		INSERT INTO instances (id, server_id, name, engine, kind, host, username, mariadb_version, port, status, labels, tags)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12)
		RETURNING created_at, updated_at, version`
	err = r.pool.QueryRow(ctx, q,
		in.ID, in.ServerID, in.Name, string(in.Engine), string(in.Kind), in.Host, in.Username,
		in.EngineVersion, in.Port, string(in.Status), string(labels), in.Tags,
	).Scan(&in.CreatedAt, &in.UpdatedAt, &in.Version)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case uniqueViolation:
				return apperr.Conflict("an instance with this name or port already exists on the server")
			case foreignKeyViolation:
				return apperr.Invalid("server_id", "server does not exist")
			}
		}
		return apperr.Internal(fmt.Errorf("insert instance: %w", err))
	}
	return nil
}

func (r *InstanceRepository) GetByID(ctx context.Context, id uuid.UUID) (*instancedom.Instance, error) {
	q := `SELECT ` + instanceColumns + ` FROM instances WHERE id = $1 AND deleted_at IS NULL`
	in, err := scanInstance(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("instance not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get instance: %w", err))
	}
	return in, nil
}

func (r *InstanceRepository) List(ctx context.Context, f instancedom.ListFilter) (instancedom.Page, error) {
	conds := []string{"deleted_at IS NULL"}
	args := make([]any, 0, 4)
	if f.ServerID != nil {
		args = append(args, *f.ServerID)
		conds = append(conds, fmt.Sprintf("server_id = $%d", len(args)))
	}
	if f.Kind != nil {
		args = append(args, string(*f.Kind))
		conds = append(conds, fmt.Sprintf("kind = $%d", len(args)))
	}
	if f.Scope != nil {
		args = append(args, idArray(f.Scope.ServerIDs))
		serverPos := len(args)
		args = append(args, idArray(f.Scope.DatabaseIDs))
		dbPos := len(args)
		conds = append(conds, fmt.Sprintf(
			"(server_id = ANY($%d) OR id IN (SELECT instance_id FROM databases WHERE id = ANY($%d)))",
			serverPos, dbPos))
	}
	args = append(args, f.Limit)
	limitPos := len(args)
	args = append(args, f.Offset)
	offsetPos := len(args)

	q := fmt.Sprintf(
		`SELECT %s, count(*) OVER() AS total
		 FROM instances WHERE %s
		 ORDER BY created_at DESC
		 LIMIT $%d OFFSET $%d`,
		instanceColumns, join(conds), limitPos, offsetPos,
	)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return instancedom.Page{}, apperr.Internal(fmt.Errorf("list instances: %w", err))
	}
	defer rows.Close()

	items := make([]*instancedom.Instance, 0)
	total := 0
	for rows.Next() {
		in, t, err := scanInstanceWithTotal(rows)
		if err != nil {
			return instancedom.Page{}, apperr.Internal(fmt.Errorf("scan instance: %w", err))
		}
		items = append(items, in)
		total = t
	}
	if err := rows.Err(); err != nil {
		return instancedom.Page{}, apperr.Internal(err)
	}
	return instancedom.Page{Items: items, Total: total}, nil
}

func (r *InstanceRepository) SetRootSecretRef(ctx context.Context, id uuid.UUID, ref string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE instances SET root_secret_ref = $2, version = version + 1 WHERE id = $1 AND deleted_at IS NULL`, id, ref)
	if err != nil {
		return apperr.Internal(fmt.Errorf("set root secret ref: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("instance not found")
	}
	return nil
}

func (r *InstanceRepository) SetStatus(ctx context.Context, id uuid.UUID, status instancedom.Status) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE instances SET status = $2, version = version + 1 WHERE id = $1 AND deleted_at IS NULL`,
		id, string(status))
	if err != nil {
		return apperr.Internal(fmt.Errorf("set instance status: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("instance not found")
	}
	return nil
}

func (r *InstanceRepository) SetContainerID(ctx context.Context, id uuid.UUID, containerID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE instances SET container_id = $2, version = version + 1 WHERE id = $1 AND deleted_at IS NULL`,
		id, containerID)
	if err != nil {
		return apperr.Internal(fmt.Errorf("set container id: %w", err))
	}
	return nil
}

func (r *InstanceRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE instances SET deleted_at = now(), status = 'deleting', version = version + 1
		 WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("soft delete instance: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("instance not found")
	}
	return nil
}

func scanInstance(row rowScanner) (*instancedom.Instance, error) {
	var (
		in           instancedom.Instance
		labelsRaw    []byte
		status       string
		engine, kind string
	)
	if err := row.Scan(
		&in.ID, &in.ServerID, &in.Name, &engine, &kind, &in.Host, &in.Username, &in.RootSecretRef,
		&in.ContainerID, &in.EngineVersion, &in.Port, &status,
		&labelsRaw, &in.Tags, &in.CreatedAt, &in.UpdatedAt, &in.Version, &in.DeletedAt,
	); err != nil {
		return nil, err
	}
	return finishInstance(&in, labelsRaw, status, engine, kind)
}

func scanInstanceWithTotal(row rowScanner) (*instancedom.Instance, int, error) {
	var (
		in           instancedom.Instance
		labelsRaw    []byte
		status       string
		engine, kind string
		total        int
	)
	if err := row.Scan(
		&in.ID, &in.ServerID, &in.Name, &engine, &kind, &in.Host, &in.Username, &in.RootSecretRef,
		&in.ContainerID, &in.EngineVersion, &in.Port, &status,
		&labelsRaw, &in.Tags, &in.CreatedAt, &in.UpdatedAt, &in.Version, &in.DeletedAt, &total,
	); err != nil {
		return nil, 0, err
	}
	out, err := finishInstance(&in, labelsRaw, status, engine, kind)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func finishInstance(in *instancedom.Instance, labelsRaw []byte, status, engine, kind string) (*instancedom.Instance, error) {
	in.Status = instancedom.Status(status)
	in.Engine = instancedom.Engine(engine)
	in.Kind = instancedom.Kind(kind)
	if len(labelsRaw) > 0 {
		if err := json.Unmarshal(labelsRaw, &in.Labels); err != nil {
			return nil, err
		}
	}
	if in.Labels == nil {
		in.Labels = map[string]string{}
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	return in, nil
}
