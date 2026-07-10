package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	auditdom "github.com/TajBrains/fleetdock/backend/internal/domain/audit"
	"github.com/TajBrains/fleetdock/backend/internal/platform/apperr"
)

// AuditRepository is the Postgres adapter for auditdom.Repository.
type AuditRepository struct {
	pool *pgxpool.Pool
}

// NewAuditRepository builds an audit repository.
func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository { return &AuditRepository{pool: pool} }

var _ auditdom.Repository = (*AuditRepository)(nil)

// Append records an entry, hash-chaining it to the previous one. The chain is
// serialized by taking the previous hash inside a transaction.
func (r *AuditRepository) Append(ctx context.Context, e *auditdom.Entry) error {
	metadata, err := json.Marshal(e.Metadata)
	if err != nil {
		return apperr.Internal(fmt.Errorf("marshal audit metadata: %w", err))
	}
	e.CreatedAt = time.Now().UTC()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return apperr.Internal(fmt.Errorf("audit begin: %w", err))
	}
	defer tx.Rollback(ctx)

	var prevHash []byte
	err = tx.QueryRow(ctx, `SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&prevHash)
	if err != nil && err != pgx.ErrNoRows {
		return apperr.Internal(fmt.Errorf("audit prev hash: %w", err))
	}

	hash := chainHash(prevHash, e, metadata)

	err = tx.QueryRow(ctx, `
		INSERT INTO audit_log (actor_type, actor_id, action, resource_type, resource_id, metadata, prev_hash, hash, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		string(e.ActorType), e.ActorID, e.Action, e.ResourceType, e.ResourceID, metadata, prevHash, hash, e.CreatedAt,
	).Scan(&e.ID)
	if err != nil {
		return apperr.Internal(fmt.Errorf("insert audit: %w", err))
	}
	return tx.Commit(ctx)
}

// chainHash = sha256(prev_hash || canonical(entry)).
func chainHash(prevHash []byte, e *auditdom.Entry, metadata []byte) []byte {
	h := sha256.New()
	h.Write(prevHash)
	canonical := struct {
		ActorType    string    `json:"actor_type"`
		ActorID      *string   `json:"actor_id"`
		Action       string    `json:"action"`
		ResourceType string    `json:"resource_type"`
		ResourceID   *string   `json:"resource_id"`
		Metadata     string    `json:"metadata"`
		CreatedAt    time.Time `json:"created_at"`
	}{
		ActorType:    string(e.ActorType),
		ActorID:      uuidPtrStr(e.ActorID),
		Action:       e.Action,
		ResourceType: e.ResourceType,
		ResourceID:   uuidPtrStr(e.ResourceID),
		Metadata:     string(metadata),
		CreatedAt:    e.CreatedAt,
	}
	b, _ := json.Marshal(canonical)
	h.Write(b)
	return h.Sum(nil)
}

func uuidPtrStr(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func (r *AuditRepository) List(ctx context.Context, f auditdom.ListFilter) (auditdom.Page, error) {
	conds := []string{"true"}
	args := make([]any, 0, 4)
	if f.ActorID != nil {
		args = append(args, *f.ActorID)
		conds = append(conds, fmt.Sprintf("actor_id = $%d", len(args)))
	}
	if f.ResourceType != "" {
		args = append(args, f.ResourceType)
		conds = append(conds, fmt.Sprintf("resource_type = $%d", len(args)))
	}
	if f.ResourceID != nil {
		args = append(args, *f.ResourceID)
		conds = append(conds, fmt.Sprintf("resource_id = $%d", len(args)))
	}
	args = append(args, f.Limit)
	limitPos := len(args)
	args = append(args, f.Offset)
	offsetPos := len(args)

	q := fmt.Sprintf(`
		SELECT id, actor_type, actor_id, action, resource_type, resource_id, metadata, created_at,
		       count(*) OVER() AS total
		FROM audit_log WHERE %s
		ORDER BY id DESC LIMIT $%d OFFSET $%d`,
		join(conds), limitPos, offsetPos)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return auditdom.Page{}, apperr.Internal(fmt.Errorf("list audit: %w", err))
	}
	defer rows.Close()

	items := make([]*auditdom.Entry, 0)
	total := 0
	for rows.Next() {
		var (
			e           auditdom.Entry
			actorType   string
			metadataRaw []byte
		)
		if err := rows.Scan(&e.ID, &actorType, &e.ActorID, &e.Action, &e.ResourceType,
			&e.ResourceID, &metadataRaw, &e.CreatedAt, &total); err != nil {
			return auditdom.Page{}, apperr.Internal(fmt.Errorf("scan audit: %w", err))
		}
		e.ActorType = auditdom.ActorType(actorType)
		if len(metadataRaw) > 0 {
			_ = json.Unmarshal(metadataRaw, &e.Metadata)
		}
		items = append(items, &e)
	}
	if err := rows.Err(); err != nil {
		return auditdom.Page{}, apperr.Internal(err)
	}
	return auditdom.Page{Items: items, Total: total}, nil
}
