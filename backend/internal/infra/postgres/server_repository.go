package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	serverdom "github.com/TajBrains/db-manager/backend/internal/domain/server"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// uniqueViolation is the PostgreSQL SQLSTATE for a unique-constraint breach.
const uniqueViolation = "23505"

// ServerRepository is the Postgres adapter implementing serverdom.Repository.
type ServerRepository struct {
	pool *pgxpool.Pool
}

// NewServerRepository builds a repository backed by the given pool.
func NewServerRepository(pool *pgxpool.Pool) *ServerRepository { return &ServerRepository{pool: pool} }

// Compile-time assertion that the adapter satisfies the port.
var _ serverdom.Repository = (*ServerRepository)(nil)

const serverColumns = `
	id, name, hostname, address::text, status, agent_version, mariadb_version, os,
	labels, tags, last_heartbeat_at, created_at, updated_at, version, deleted_at`

func (r *ServerRepository) Create(ctx context.Context, s *serverdom.Server) error {
	labels, err := json.Marshal(s.Labels)
	if err != nil {
		return apperr.Internal(fmt.Errorf("marshal labels: %w", err))
	}

	const q = `
		INSERT INTO servers (id, name, hostname, address, status, os, labels, tags)
		VALUES ($1, $2, $3, $4::inet, $5, $6, $7::jsonb, $8)
		RETURNING created_at, updated_at, version`

	err = r.pool.QueryRow(ctx, q,
		s.ID, s.Name, s.Hostname, s.Address, string(s.Status), s.OS, string(labels), s.Tags,
	).Scan(&s.CreatedAt, &s.UpdatedAt, &s.Version)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return apperr.Conflict("a server with this name already exists")
		}
		return apperr.Internal(fmt.Errorf("insert server: %w", err))
	}
	return nil
}

func (r *ServerRepository) GetByID(ctx context.Context, id uuid.UUID) (*serverdom.Server, error) {
	q := `SELECT ` + serverColumns + ` FROM servers WHERE id = $1 AND deleted_at IS NULL`
	s, err := scanServer(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("server not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get server: %w", err))
	}
	return s, nil
}

func (r *ServerRepository) List(ctx context.Context, f serverdom.ListFilter) (serverdom.Page, error) {
	conds := []string{"deleted_at IS NULL"}
	args := make([]any, 0, 5)

	if f.Status != nil {
		args = append(args, string(*f.Status))
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		conds = append(conds, fmt.Sprintf("(name ILIKE $%d OR hostname ILIKE $%d)", len(args), len(args)))
	}
	if f.Tag != "" {
		args = append(args, f.Tag)
		conds = append(conds, fmt.Sprintf("$%d = ANY(tags)", len(args)))
	}
	if f.Scope != nil {
		// Only server-scoped grants surface a server in the list.
		args = append(args, idArray(f.Scope.ServerIDs))
		conds = append(conds, fmt.Sprintf("id = ANY($%d)", len(args)))
	}

	args = append(args, f.Limit)
	limitPos := len(args)
	args = append(args, f.Offset)
	offsetPos := len(args)

	q := fmt.Sprintf(
		`SELECT %s, count(*) OVER() AS total
		 FROM servers
		 WHERE %s
		 ORDER BY created_at DESC
		 LIMIT $%d OFFSET $%d`,
		serverColumns, strings.Join(conds, " AND "), limitPos, offsetPos,
	)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return serverdom.Page{}, apperr.Internal(fmt.Errorf("list servers: %w", err))
	}
	defer rows.Close()

	items := make([]*serverdom.Server, 0)
	total := 0
	for rows.Next() {
		s, t, err := scanServerWithTotal(rows)
		if err != nil {
			return serverdom.Page{}, apperr.Internal(fmt.Errorf("scan server: %w", err))
		}
		items = append(items, s)
		total = t
	}
	if err := rows.Err(); err != nil {
		return serverdom.Page{}, apperr.Internal(fmt.Errorf("iterate servers: %w", err))
	}
	return serverdom.Page{Items: items, Total: total}, nil
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanServer(row rowScanner) (*serverdom.Server, error) {
	var (
		s         serverdom.Server
		labelsRaw []byte
		status    string
	)
	if err := row.Scan(
		&s.ID, &s.Name, &s.Hostname, &s.Address, &status, &s.AgentVersion, &s.MariaDBVersion, &s.OS,
		&labelsRaw, &s.Tags, &s.LastHeartbeatAt, &s.CreatedAt, &s.UpdatedAt, &s.Version, &s.DeletedAt,
	); err != nil {
		return nil, err
	}
	return finishServer(&s, labelsRaw, status)
}

func scanServerWithTotal(row rowScanner) (*serverdom.Server, int, error) {
	var (
		s         serverdom.Server
		labelsRaw []byte
		status    string
		total     int
	)
	if err := row.Scan(
		&s.ID, &s.Name, &s.Hostname, &s.Address, &status, &s.AgentVersion, &s.MariaDBVersion, &s.OS,
		&labelsRaw, &s.Tags, &s.LastHeartbeatAt, &s.CreatedAt, &s.UpdatedAt, &s.Version, &s.DeletedAt,
		&total,
	); err != nil {
		return nil, 0, err
	}
	out, err := finishServer(&s, labelsRaw, status)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func finishServer(s *serverdom.Server, labelsRaw []byte, status string) (*serverdom.Server, error) {
	s.Status = serverdom.Status(status)
	if len(labelsRaw) > 0 {
		if err := json.Unmarshal(labelsRaw, &s.Labels); err != nil {
			return nil, err
		}
	}
	if s.Labels == nil {
		s.Labels = map[string]string{}
	}
	if s.Tags == nil {
		s.Tags = []string{}
	}
	return s, nil
}
