package postgres

import (
	"context"
	"fmt"

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
		INSERT INTO audit_events (id, actor_user_id, action, resource_type, resource_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
		RETURNING created_at`
	meta := e.Metadata
	if meta == nil {
		meta = []byte("{}")
	}
	err := r.pool.QueryRow(ctx, q, e.ID, e.ActorUserID, e.Action, e.ResourceType, e.ResourceID, meta).
		Scan(&e.CreatedAt)
	if err != nil {
		return apperr.Internal(fmt.Errorf("append audit event: %w", err))
	}
	return nil
}
