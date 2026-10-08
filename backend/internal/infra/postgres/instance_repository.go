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

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
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
	labels, tags, created_at, updated_at, version, deleted_at, tls_mode, health,
	ssh_host, ssh_port, ssh_user, ssh_auth, ssh_secret_ref, ssh_host_key`

func (r *InstanceRepository) Create(ctx context.Context, in *instancedom.Instance) error {
	labels, err := json.Marshal(in.Labels)
	if err != nil {
		return apperr.Internal(fmt.Errorf("marshal labels: %w", err))
	}
	const q = `
		INSERT INTO instances (id, server_id, name, engine, kind, host, username, mariadb_version, port, status, labels, tags, tls_mode,
		                       ssh_host, ssh_port, ssh_user, ssh_auth, ssh_secret_ref, ssh_host_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING created_at, updated_at, version`
	ssh := sshColumns(in.SSH)
	err = r.pool.QueryRow(ctx, q,
		in.ID, in.ServerID, in.Name, string(in.Engine), string(in.Kind), in.Host, in.Username,
		in.EngineVersion, in.Port, string(in.Status), string(labels), in.Tags, in.TLSModeOrDefault(),
		ssh.host, ssh.port, ssh.user, ssh.auth, ssh.secretRef, ssh.hostKey,
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

func (r *InstanceRepository) Update(ctx context.Context, id uuid.UUID, f instancedom.UpdateFields) error {
	sets := make([]string, 0, 5)
	args := make([]any, 0, 6)
	args = append(args, id)

	set := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if f.Name != nil {
		set("name", *f.Name)
	}
	if f.Host != nil {
		set("host", *f.Host)
	}
	if f.Port != nil {
		set("port", *f.Port)
	}
	if f.TLSMode != nil {
		set("tls_mode", *f.TLSMode)
	}
	if f.Credentials != nil {
		set("username", f.Credentials.Username)
		set("root_secret_ref", f.Credentials.RootSecretRef)
	}
	switch {
	case f.RemoveSSH:
		set("ssh_host", nil)
		set("ssh_port", 22)
		set("ssh_user", nil)
		set("ssh_auth", nil)
		set("ssh_secret_ref", nil)
		set("ssh_host_key", nil)
	case f.SSH != nil:
		ssh := sshColumns(f.SSH)
		set("ssh_host", ssh.host)
		set("ssh_port", ssh.port)
		set("ssh_user", ssh.user)
		set("ssh_auth", ssh.auth)
		set("ssh_secret_ref", ssh.secretRef)
		set("ssh_host_key", ssh.hostKey)
	case f.ResetSSHHostKey:
		set("ssh_host_key", nil)
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = now()", "version = version + 1")

	q := fmt.Sprintf(`UPDATE instances SET %s WHERE id = $1 AND deleted_at IS NULL`, joinSet(sets))
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("an instance with this name or port already exists on the server")
		}
		return apperr.Internal(fmt.Errorf("update instance: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("instance not found")
	}
	return nil
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
		healthRaw    []byte
		status       string
		engine, kind string
		ssh          sshRow
	)
	if err := row.Scan(
		&in.ID, &in.ServerID, &in.Name, &engine, &kind, &in.Host, &in.Username, &in.RootSecretRef,
		&in.ContainerID, &in.EngineVersion, &in.Port, &status,
		&labelsRaw, &in.Tags, &in.CreatedAt, &in.UpdatedAt, &in.Version, &in.DeletedAt, &in.TLSMode, &healthRaw,
		&ssh.host, &ssh.port, &ssh.user, &ssh.auth, &ssh.secretRef, &ssh.hostKey,
	); err != nil {
		return nil, err
	}
	in.SSH = ssh.tunnel()
	return finishInstance(&in, labelsRaw, healthRaw, status, engine, kind)
}

func scanInstanceWithTotal(row rowScanner) (*instancedom.Instance, int, error) {
	var (
		in           instancedom.Instance
		labelsRaw    []byte
		healthRaw    []byte
		status       string
		engine, kind string
		ssh          sshRow
		total        int
	)
	if err := row.Scan(
		&in.ID, &in.ServerID, &in.Name, &engine, &kind, &in.Host, &in.Username, &in.RootSecretRef,
		&in.ContainerID, &in.EngineVersion, &in.Port, &status,
		&labelsRaw, &in.Tags, &in.CreatedAt, &in.UpdatedAt, &in.Version, &in.DeletedAt, &in.TLSMode, &healthRaw,
		&ssh.host, &ssh.port, &ssh.user, &ssh.auth, &ssh.secretRef, &ssh.hostKey, &total,
	); err != nil {
		return nil, 0, err
	}
	in.SSH = ssh.tunnel()
	out, err := finishInstance(&in, labelsRaw, healthRaw, status, engine, kind)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func finishInstance(in *instancedom.Instance, labelsRaw, healthRaw []byte, status, engine, kind string) (*instancedom.Instance, error) {
	if len(healthRaw) > 0 {
		var h instancedom.Health
		if err := json.Unmarshal(healthRaw, &h); err == nil {
			in.Health = &h
		}
	}
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

func (r *InstanceRepository) SetHealth(ctx context.Context, id uuid.UUID, h instancedom.Health) error {
	raw, err := json.Marshal(h)
	if err != nil {
		return apperr.Internal(err)
	}
	// Deliberately no version bump: health is observed state, and bumping
	// would make every probe look like a user edit.
	if _, err := r.pool.Exec(ctx, `UPDATE instances SET health = $2::jsonb WHERE id = $1`, id, string(raw)); err != nil {
		return apperr.Internal(fmt.Errorf("set instance health: %w", err))
	}
	return nil
}

func (r *InstanceRepository) PinSSHHostKey(ctx context.Context, id uuid.UUID, key string) error {
	// No version bump, like health: pinning is observed state, not a user edit.
	if _, err := r.pool.Exec(ctx,
		`UPDATE instances SET ssh_host_key = $2
		 WHERE id = $1 AND ssh_host IS NOT NULL AND ssh_host_key IS NULL AND deleted_at IS NULL`,
		id, key); err != nil {
		return apperr.Internal(fmt.Errorf("pin ssh host key: %w", err))
	}
	return nil
}

// sshRow is the nullable column form of an instance's SSH tunnel.
type sshRow struct {
	host, user, auth, secretRef, hostKey *string
	port                                 int
}

func sshColumns(t *instancedom.SSHTunnel) sshRow {
	if t == nil {
		return sshRow{port: 22}
	}
	auth := string(t.Auth)
	row := sshRow{host: &t.Host, port: t.Port, user: &t.User, auth: &auth, secretRef: t.SecretRef}
	if t.HostKey != "" {
		row.hostKey = &t.HostKey
	}
	return row
}

func (s sshRow) tunnel() *instancedom.SSHTunnel {
	if s.host == nil {
		return nil
	}
	t := &instancedom.SSHTunnel{Host: *s.host, Port: s.port, SecretRef: s.secretRef}
	if s.user != nil {
		t.User = *s.user
	}
	if s.auth != nil {
		t.Auth = instancedom.SSHAuth(*s.auth)
	}
	if s.hostKey != nil {
		t.HostKey = *s.hostKey
	}
	return t
}
