package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	notifdom "github.com/TajBrains/db-manager/backend/internal/domain/notification"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// NotificationRepository is the Postgres adapter for notifdom.Repository.
type NotificationRepository struct {
	pool *pgxpool.Pool
}

// NewNotificationRepository builds the repository.
func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

var _ notifdom.Repository = (*NotificationRepository)(nil)

// ---- Channels ----

const channelColumns = `id, name, type, config, enabled, created_at, updated_at, version`

func (r *NotificationRepository) CreateChannel(ctx context.Context, c *notifdom.Channel) error {
	config, _ := json.Marshal(c.Config)
	const q = `
		INSERT INTO notification_channels (id, name, type, config, enabled)
		VALUES ($1, $2, $3, $4::jsonb, $5)
		RETURNING created_at, updated_at, version`
	err := r.pool.QueryRow(ctx, q, c.ID, c.Name, string(c.Type), string(config), c.Enabled).
		Scan(&c.CreatedAt, &c.UpdatedAt, &c.Version)
	if err != nil {
		return apperr.Internal(fmt.Errorf("insert channel: %w", err))
	}
	return nil
}

func (r *NotificationRepository) GetChannel(ctx context.Context, id uuid.UUID) (*notifdom.Channel, error) {
	q := `SELECT ` + channelColumns + ` FROM notification_channels WHERE id = $1`
	c, err := scanChannel(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("notification channel not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get channel: %w", err))
	}
	return c, nil
}

func (r *NotificationRepository) ListChannels(ctx context.Context) ([]*notifdom.Channel, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+channelColumns+` FROM notification_channels ORDER BY created_at DESC`)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list channels: %w", err))
	}
	defer rows.Close()
	return collectChannels(rows)
}

func (r *NotificationRepository) ListEnabledChannels(ctx context.Context, ids []uuid.UUID) ([]*notifdom.Channel, error) {
	q := `SELECT ` + channelColumns + ` FROM notification_channels WHERE enabled`
	args := []any{}
	if len(ids) > 0 {
		q += ` AND id = ANY($1)`
		args = append(args, ids)
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list enabled channels: %w", err))
	}
	defer rows.Close()
	return collectChannels(rows)
}

func (r *NotificationRepository) UpdateChannel(ctx context.Context, c *notifdom.Channel) error {
	config, _ := json.Marshal(c.Config)
	const q = `
		UPDATE notification_channels SET name = $2, type = $3, config = $4::jsonb, enabled = $5, version = version + 1
		WHERE id = $1 RETURNING updated_at, version`
	err := r.pool.QueryRow(ctx, q, c.ID, c.Name, string(c.Type), string(config), c.Enabled).
		Scan(&c.UpdatedAt, &c.Version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("notification channel not found")
		}
		return apperr.Internal(fmt.Errorf("update channel: %w", err))
	}
	return nil
}

func (r *NotificationRepository) DeleteChannel(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM notification_channels WHERE id = $1`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("delete channel: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("notification channel not found")
	}
	return nil
}

func collectChannels(rows pgx.Rows) ([]*notifdom.Channel, error) {
	out := make([]*notifdom.Channel, 0)
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanChannel(row rowScanner) (*notifdom.Channel, error) {
	var (
		c         notifdom.Channel
		typ       string
		configRaw []byte
	)
	if err := row.Scan(&c.ID, &c.Name, &typ, &configRaw, &c.Enabled, &c.CreatedAt, &c.UpdatedAt, &c.Version); err != nil {
		return nil, err
	}
	c.Type = notifdom.ChannelType(typ)
	if len(configRaw) > 0 {
		_ = json.Unmarshal(configRaw, &c.Config)
	}
	if c.Config == nil {
		c.Config = map[string]string{}
	}
	return &c, nil
}

// ---- Rules ----

const ruleColumns = `id, name, target_type, target_id, metric, comparator, threshold, for_seconds, severity, channel_ids, enabled, created_at, updated_at, version`

func (r *NotificationRepository) CreateRule(ctx context.Context, ru *notifdom.Rule) error {
	const q = `
		INSERT INTO alert_rules (id, name, target_type, target_id, metric, comparator, threshold, for_seconds, severity, channel_ids, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at, version`
	err := r.pool.QueryRow(ctx, q,
		ru.ID, ru.Name, ru.TargetType, ru.TargetID, string(ru.Metric), string(ru.Comparator),
		ru.Threshold, ru.ForSeconds, ru.Severity, ru.ChannelIDs, ru.Enabled,
	).Scan(&ru.CreatedAt, &ru.UpdatedAt, &ru.Version)
	if err != nil {
		return apperr.Internal(fmt.Errorf("insert rule: %w", err))
	}
	return nil
}

func (r *NotificationRepository) GetRule(ctx context.Context, id uuid.UUID) (*notifdom.Rule, error) {
	q := `SELECT ` + ruleColumns + ` FROM alert_rules WHERE id = $1`
	ru, err := scanRule(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("alert rule not found")
		}
		return nil, apperr.Internal(fmt.Errorf("get rule: %w", err))
	}
	return ru, nil
}

func (r *NotificationRepository) ListRules(ctx context.Context) ([]*notifdom.Rule, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+ruleColumns+` FROM alert_rules ORDER BY created_at DESC`)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list rules: %w", err))
	}
	defer rows.Close()
	return collectRules(rows)
}

func (r *NotificationRepository) ListEnabledRules(ctx context.Context) ([]*notifdom.Rule, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+ruleColumns+` FROM alert_rules WHERE enabled ORDER BY created_at`)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list enabled rules: %w", err))
	}
	defer rows.Close()
	return collectRules(rows)
}

func (r *NotificationRepository) UpdateRule(ctx context.Context, ru *notifdom.Rule) error {
	const q = `
		UPDATE alert_rules SET name = $2, target_type = $3, target_id = $4, metric = $5, comparator = $6,
			threshold = $7, for_seconds = $8, severity = $9, channel_ids = $10, enabled = $11, version = version + 1
		WHERE id = $1 RETURNING updated_at, version`
	err := r.pool.QueryRow(ctx, q,
		ru.ID, ru.Name, ru.TargetType, ru.TargetID, string(ru.Metric), string(ru.Comparator),
		ru.Threshold, ru.ForSeconds, ru.Severity, ru.ChannelIDs, ru.Enabled,
	).Scan(&ru.UpdatedAt, &ru.Version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("alert rule not found")
		}
		return apperr.Internal(fmt.Errorf("update rule: %w", err))
	}
	return nil
}

func (r *NotificationRepository) DeleteRule(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM alert_rules WHERE id = $1`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("delete rule: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound("alert rule not found")
	}
	return nil
}

func collectRules(rows pgx.Rows) ([]*notifdom.Rule, error) {
	out := make([]*notifdom.Rule, 0)
	for rows.Next() {
		ru, err := scanRule(rows)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		out = append(out, ru)
	}
	return out, rows.Err()
}

func scanRule(row rowScanner) (*notifdom.Rule, error) {
	var (
		ru  notifdom.Rule
		met string
		cmp string
	)
	if err := row.Scan(&ru.ID, &ru.Name, &ru.TargetType, &ru.TargetID, &met, &cmp,
		&ru.Threshold, &ru.ForSeconds, &ru.Severity, &ru.ChannelIDs, &ru.Enabled,
		&ru.CreatedAt, &ru.UpdatedAt, &ru.Version); err != nil {
		return nil, err
	}
	ru.Metric = notifdom.Metric(met)
	ru.Comparator = notifdom.Comparator(cmp)
	if ru.ChannelIDs == nil {
		ru.ChannelIDs = []uuid.UUID{}
	}
	return &ru, nil
}

// ---- Outbox ----

type outboxPayload struct {
	Title      string      `json:"title"`
	Message    string      `json:"message"`
	Severity   string      `json:"severity"`
	ChannelIDs []uuid.UUID `json:"channel_ids"`
}

func (r *NotificationRepository) Enqueue(ctx context.Context, e notifdom.Event) error {
	payload, _ := json.Marshal(outboxPayload{
		Title: e.Title, Message: e.Message, Severity: e.Severity, ChannelIDs: e.ChannelIDs,
	})
	_, err := r.pool.Exec(ctx, `
		INSERT INTO outbox (aggregate_type, aggregate_id, event_type, payload)
		VALUES ($1, $2, $3, $4::jsonb)`,
		e.AggregateType, e.AggregateID, e.EventType, string(payload))
	if err != nil {
		return apperr.Internal(fmt.Errorf("enqueue event: %w", err))
	}
	return nil
}

func (r *NotificationRepository) ListUnpublished(ctx context.Context, limit int) ([]notifdom.OutboxRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, aggregate_type, aggregate_id, event_type, payload, attempts
		FROM outbox WHERE published_at IS NULL AND attempts < 5
		ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list outbox: %w", err))
	}
	defer rows.Close()
	out := make([]notifdom.OutboxRow, 0)
	for rows.Next() {
		var (
			row        notifdom.OutboxRow
			payloadRaw []byte
			p          outboxPayload
		)
		if err := rows.Scan(&row.ID, &row.Event.AggregateType, &row.Event.AggregateID,
			&row.Event.EventType, &payloadRaw, &row.Attempts); err != nil {
			return nil, apperr.Internal(fmt.Errorf("scan outbox: %w", err))
		}
		_ = json.Unmarshal(payloadRaw, &p)
		row.Event.Title, row.Event.Message, row.Event.Severity = p.Title, p.Message, p.Severity
		row.Event.ChannelIDs = p.ChannelIDs
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *NotificationRepository) MarkPublished(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE outbox SET published_at = now(), attempts = attempts + 1 WHERE id = $1`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("mark published: %w", err))
	}
	return nil
}

func (r *NotificationRepository) MarkFailed(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE outbox SET attempts = attempts + 1 WHERE id = $1`, id)
	if err != nil {
		return apperr.Internal(fmt.Errorf("mark failed: %w", err))
	}
	return nil
}
