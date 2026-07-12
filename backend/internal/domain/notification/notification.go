// Package notification is the domain model for notification channels
// (email/slack/webhook), alert rules, and the outbox events that connect
// them.
package notification

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// ChannelType is the delivery mechanism of a notification channel.
type ChannelType string

const (
	ChannelEmail   ChannelType = "email"
	ChannelSlack   ChannelType = "slack"
	ChannelWebhook ChannelType = "webhook"
)

// Valid reports whether t is a known channel type.
func (t ChannelType) Valid() bool {
	switch t {
	case ChannelEmail, ChannelSlack, ChannelWebhook:
		return true
	}
	return false
}

// Channel is a configured notification target.
type Channel struct {
	ID        uuid.UUID
	Name      string
	Type      ChannelType
	Config    map[string]string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// NewChannel validates and constructs a channel.
func NewChannel(name string, t ChannelType, config map[string]string, enabled bool) (*Channel, error) {
	if err := validateChannel(name, t, config); err != nil {
		return nil, err
	}
	if config == nil {
		config = map[string]string{}
	}
	return &Channel{ID: uuid.New(), Name: name, Type: t, Config: config, Enabled: enabled}, nil
}

// Apply validates and overwrites mutable channel fields.
func (c *Channel) Apply(name string, t ChannelType, config map[string]string, enabled bool) error {
	if err := validateChannel(name, t, config); err != nil {
		return err
	}
	c.Name, c.Type, c.Config, c.Enabled = name, t, config, enabled
	if c.Config == nil {
		c.Config = map[string]string{}
	}
	return nil
}

func validateChannel(name string, t ChannelType, config map[string]string) error {
	if len(name) == 0 || len(name) > 63 {
		return apperr.Invalid("name", "name is required and must be at most 63 characters")
	}
	if !t.Valid() {
		return apperr.Invalid("type", "type must be one of: email, slack, webhook")
	}
	switch t {
	case ChannelWebhook:
		if config["url"] == "" {
			return apperr.Invalid("config", "webhook channels require a config.url")
		}
	case ChannelSlack:
		if config["webhook_url"] == "" {
			return apperr.Invalid("config", "slack channels require a config.webhook_url")
		}
	case ChannelEmail:
		if config["to"] == "" {
			return apperr.Invalid("config", "email channels require a config.to address")
		}
	}
	return nil
}

// Comparator is how an alert threshold is applied.
type Comparator string

const (
	CmpGt  Comparator = "gt"
	CmpGte Comparator = "gte"
	CmpLt  Comparator = "lt"
	CmpLte Comparator = "lte"
)

// Compare reports whether value satisfies `value <cmp> threshold`.
func (c Comparator) Compare(value, threshold float64) bool {
	switch c {
	case CmpGt:
		return value > threshold
	case CmpGte:
		return value >= threshold
	case CmpLt:
		return value < threshold
	case CmpLte:
		return value <= threshold
	}
	return false
}

// Valid reports whether c is a known comparator.
func (c Comparator) Valid() bool {
	switch c {
	case CmpGt, CmpGte, CmpLt, CmpLte:
		return true
	}
	return false
}

// Metric names an alertable measurement.
type Metric string

const (
	MetricCPUPct      Metric = "cpu_pct"
	MetricMemUsedPct  Metric = "mem_used_pct"
	MetricDiskUsedPct Metric = "disk_used_pct"
	MetricConnections Metric = "connections"
)

// Valid reports whether m is a known metric.
func (m Metric) Valid() bool {
	switch m {
	case MetricCPUPct, MetricMemUsedPct, MetricDiskUsedPct, MetricConnections:
		return true
	}
	return false
}

// Rule is an alert rule evaluated against server metrics.
type Rule struct {
	ID         uuid.UUID
	Name       string
	TargetType string // server | global (instance/database reserved)
	TargetID   *uuid.UUID
	Metric     Metric
	Comparator Comparator
	Threshold  float64
	ForSeconds int
	Severity   string // info | warning | critical
	ChannelIDs []uuid.UUID
	Enabled    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Version    int
}

// NewRule validates and constructs an alert rule.
func NewRule(name, targetType string, targetID *uuid.UUID, metric Metric, cmp Comparator, threshold float64, forSeconds int, severity string, channelIDs []uuid.UUID, enabled bool) (*Rule, error) {
	r := &Rule{ID: uuid.New()}
	if err := r.set(name, targetType, targetID, metric, cmp, threshold, forSeconds, severity, channelIDs, enabled); err != nil {
		return nil, err
	}
	return r, nil
}

// Apply validates and overwrites mutable rule fields.
func (r *Rule) Apply(name, targetType string, targetID *uuid.UUID, metric Metric, cmp Comparator, threshold float64, forSeconds int, severity string, channelIDs []uuid.UUID, enabled bool) error {
	return r.set(name, targetType, targetID, metric, cmp, threshold, forSeconds, severity, channelIDs, enabled)
}

func (r *Rule) set(name, targetType string, targetID *uuid.UUID, metric Metric, cmp Comparator, threshold float64, forSeconds int, severity string, channelIDs []uuid.UUID, enabled bool) error {
	if len(name) == 0 || len(name) > 63 {
		return apperr.Invalid("name", "name is required and must be at most 63 characters")
	}
	if targetType != "server" && targetType != "global" {
		return apperr.Invalid("target_type", "target_type must be 'server' or 'global'")
	}
	if targetType == "server" && targetID == nil {
		return apperr.Invalid("target_id", "target_id is required for server-scoped rules")
	}
	if !metric.Valid() {
		return apperr.Invalid("metric", "metric must be one of: cpu_pct, mem_used_pct, disk_used_pct, connections")
	}
	if !cmp.Valid() {
		return apperr.Invalid("comparator", "comparator must be one of: gt, gte, lt, lte")
	}
	if forSeconds < 0 {
		return apperr.Invalid("for_seconds", "for_seconds must be zero or positive")
	}
	switch severity {
	case "info", "warning", "critical":
	default:
		return apperr.Invalid("severity", "severity must be one of: info, warning, critical")
	}
	if channelIDs == nil {
		channelIDs = []uuid.UUID{}
	}
	r.Name, r.TargetType, r.TargetID = name, targetType, targetID
	r.Metric, r.Comparator, r.Threshold = metric, cmp, threshold
	r.ForSeconds, r.Severity, r.ChannelIDs, r.Enabled = forSeconds, severity, channelIDs, enabled
	return nil
}

// Event is a domain event queued in the outbox for delivery.
type Event struct {
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	Title         string
	Message       string
	Severity      string
	ChannelIDs    []uuid.UUID // empty = all enabled channels
}

// OutboxRow is a persisted, not-yet-delivered event.
type OutboxRow struct {
	ID       int64
	Event    Event
	Attempts int
}

// Repository is the persistence port for channels, rules and the outbox.
type Repository interface {
	// Channels
	CreateChannel(ctx context.Context, c *Channel) error
	GetChannel(ctx context.Context, id uuid.UUID) (*Channel, error)
	ListChannels(ctx context.Context) ([]*Channel, error)
	ListEnabledChannels(ctx context.Context, ids []uuid.UUID) ([]*Channel, error)
	UpdateChannel(ctx context.Context, c *Channel) error
	DeleteChannel(ctx context.Context, id uuid.UUID) error

	// Rules
	CreateRule(ctx context.Context, r *Rule) error
	GetRule(ctx context.Context, id uuid.UUID) (*Rule, error)
	ListRules(ctx context.Context) ([]*Rule, error)
	ListEnabledRules(ctx context.Context) ([]*Rule, error)
	UpdateRule(ctx context.Context, r *Rule) error
	DeleteRule(ctx context.Context, id uuid.UUID) error

	// Outbox
	Enqueue(ctx context.Context, e Event) error
	ListUnpublished(ctx context.Context, limit int) ([]OutboxRow, error)
	MarkPublished(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64) error
}
