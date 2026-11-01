package postgres

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auditdom "github.com/Fleetdock/fleetdock/backend/internal/domain/audit"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// AuditRepository is the Postgres adapter for auditdom.Repository.
type AuditRepository struct {
	pool *pgxpool.Pool
}

// NewAuditRepository builds an audit repository.
func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{pool: pool}
}

var _ auditdom.Repository = (*AuditRepository)(nil)

func (r *AuditRepository) Append(ctx context.Context, e *auditdom.Event) error {
	const q = `
		INSERT INTO audit_events (id, actor_user_id, actor_email, actor_kind, token_id, ip, user_agent,
			action, outcome, resource_type, resource_id, resource_name, metadata)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, NULLIF($7, ''), $8, $9, $10, $11, NULLIF($12, ''), $13::jsonb)
		RETURNING created_at`
	meta := e.Metadata
	if meta == nil {
		meta = []byte("{}")
	}
	var ip *netip.Addr
	if a, err := netip.ParseAddr(e.IP); err == nil {
		ip = &a
	}
	err := r.pool.QueryRow(ctx, q, e.ID, e.ActorUserID, e.ActorEmail, e.ActorKind, e.TokenID, ip, e.UserAgent,
		e.Action, e.Outcome, e.ResourceType, e.ResourceID, e.ResourceName, meta).
		Scan(&e.CreatedAt)
	if err != nil {
		return apperr.Internal(fmt.Errorf("append audit event: %w", err))
	}
	return nil
}

const auditColumns = `a.id, a.actor_user_id, COALESCE(a.actor_email, u.email, ''), a.actor_kind, a.token_id,
	a.ip, COALESCE(a.user_agent, ''), a.action, a.outcome, a.resource_type, a.resource_id,
	COALESCE(a.resource_name, ''), a.metadata, a.created_at`

// List returns events newest first, paged by (created_at, id).
func (r *AuditRepository) List(ctx context.Context, f auditdom.Filter) ([]auditdom.Event, error) {
	q := `SELECT ` + auditColumns + ` FROM audit_events a LEFT JOIN users u ON u.id = a.actor_user_id WHERE true`
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		q += " AND " + strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args)))
	}
	if f.ActorUserID != nil {
		add("a.actor_user_id = ?", *f.ActorUserID)
	}
	if f.ActionPrefix != "" {
		if strings.HasSuffix(f.ActionPrefix, ".") {
			add("starts_with(a.action, ?)", f.ActionPrefix)
		} else {
			add("a.action = ?", f.ActionPrefix)
		}
	}
	if f.ResourceType != "" {
		add("a.resource_type = ?", f.ResourceType)
	}
	if f.ResourceID != nil {
		add("a.resource_id = ?", *f.ResourceID)
	}
	if f.Outcome != "" {
		add("a.outcome = ?", f.Outcome)
	}
	if !f.Since.IsZero() {
		add("a.created_at >= ?", f.Since)
	}
	if !f.Until.IsZero() {
		add("a.created_at < ?", f.Until)
	}
	if f.After != nil {
		args = append(args, f.After.CreatedAt, f.After.ID)
		q += fmt.Sprintf(" AND (a.created_at, a.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit)
	q += fmt.Sprintf(" ORDER BY a.created_at DESC, a.id DESC LIMIT $%d", len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list audit events: %w", err))
	}
	defer rows.Close()
	out := []auditdom.Event{}
	for rows.Next() {
		var e auditdom.Event
		var ip *netip.Addr
		if err := rows.Scan(&e.ID, &e.ActorUserID, &e.ActorEmail, &e.ActorKind, &e.TokenID, &ip, &e.UserAgent,
			&e.Action, &e.Outcome, &e.ResourceType, &e.ResourceID, &e.ResourceName, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, apperr.Internal(fmt.Errorf("scan audit event: %w", err))
		}
		if ip != nil {
			e.IP = ip.String()
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// PruneBefore deletes expired events in batches. Each batch runs in its own
// transaction that sets fleetdock.audit_prune, the one way past the table's
// append-only trigger.
func (r *AuditRepository) PruneBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	var total int64
	for {
		var n int64
		err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('fleetdock.audit_prune', 'on', true)`); err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `DELETE FROM audit_events WHERE id IN (
				SELECT id FROM audit_events WHERE created_at < $1 LIMIT 5000)`, cutoff)
			n = tag.RowsAffected()
			return err
		})
		if err != nil {
			return total, apperr.Internal(fmt.Errorf("prune audit events: %w", err))
		}
		total += n
		if n < 5000 {
			return total, nil
		}
	}
}

// resourceNames maps a resource type to the query naming one resource.
var resourceNames = map[string]string{
	"user":                 `SELECT email FROM users WHERE id = $1`,
	"role":                 `SELECT name FROM roles WHERE id = $1`,
	"server":               `SELECT COALESCE(NULLIF(name, ''), hostname) FROM servers WHERE id = $1`,
	"instance":             `SELECT name FROM instances WHERE id = $1`,
	"database":             `SELECT name FROM databases WHERE id = $1`,
	"backup":               `SELECT d.name FROM backups b JOIN databases d ON d.id = b.database_id WHERE b.id = $1`,
	"backup_schedule":      `SELECT d.name FROM backup_schedules s JOIN databases d ON d.id = s.database_id WHERE s.id = $1`,
	"backup_destination":   `SELECT name FROM backup_destinations WHERE id = $1`,
	"notification_channel": `SELECT name FROM notification_channels WHERE id = $1`,
	"alert_rule":           `SELECT name FROM alert_rules WHERE id = $1`,
	"api_token":            `SELECT name FROM api_tokens WHERE id = $1`,
	"agent_token":          `SELECT name FROM agent_registration_tokens WHERE id = $1`,
}

// ResourceName looks up a resource's display name; "" when unknown.
func (r *AuditRepository) ResourceName(ctx context.Context, resourceType string, id uuid.UUID) string {
	q, ok := resourceNames[resourceType]
	if !ok {
		return ""
	}
	var name *string
	if err := r.pool.QueryRow(ctx, q, id).Scan(&name); err != nil || name == nil {
		return ""
	}
	return *name
}
