// Package notificationapp implements notification channels, alert rules, the
// outbox dispatcher and the alert evaluator.
package notificationapp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	notifdom "github.com/Fleetdock/fleetdock/backend/internal/domain/notification"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/notify"
)

// MetricsSource provides the latest server health for alert evaluation.
type MetricsSource interface {
	LatestServerHealth(ctx context.Context) ([]serverdom.HealthSample, error)
}

// Service implements notification use cases.
type Service struct {
	repo    notifdom.Repository
	sender  *notify.Sender
	metrics MetricsSource

	mu        sync.Mutex
	firing    map[string]bool      // ruleID/serverID -> currently firing
	breaching map[string]time.Time // ruleID/serverID -> first breach time
}

// NewService wires the notification service.
func NewService(repo notifdom.Repository, sender *notify.Sender, metrics MetricsSource) *Service {
	return &Service{
		repo: repo, sender: sender, metrics: metrics,
		firing:    map[string]bool{},
		breaching: map[string]time.Time{},
	}
}

// ---- Channels ----

// ChannelInput describes a channel create/update.
type ChannelInput struct {
	Name    string
	Type    string
	Config  map[string]string
	Enabled bool
}

// CreateChannel validates and persists a channel.
func (s *Service) CreateChannel(ctx context.Context, in ChannelInput) (*notifdom.Channel, error) {
	c, err := notifdom.NewChannel(in.Name, notifdom.ChannelType(in.Type), in.Config, in.Enabled)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateChannel(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// RedactedMask is the placeholder returned in place of secret config values.
const RedactedMask = "••••••"

// secretConfigKeys hold sensitive values that are masked on read; when an
// update sends them blank or masked, the stored value is preserved.
var secretConfigKeys = map[string]bool{"url": true, "webhook_url": true, "password": true}

// UpdateChannel validates and persists channel changes.
func (s *Service) UpdateChannel(ctx context.Context, id string, in ChannelInput) (*notifdom.Channel, error) {
	c, err := s.getChannel(ctx, id)
	if err != nil {
		return nil, err
	}
	config := mergeSecretConfig(in.Config, c.Config)
	if err := c.Apply(in.Name, notifdom.ChannelType(in.Type), config, in.Enabled); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateChannel(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// mergeSecretConfig carries forward stored secret values when the incoming
// config leaves them blank or masked.
func mergeSecretConfig(incoming, existing map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range incoming {
		if secretConfigKeys[k] && (v == "" || v == RedactedMask) {
			if ev, ok := existing[k]; ok {
				out[k] = ev
				continue
			}
		}
		out[k] = v
	}
	return out
}

// ListChannels returns all channels.
func (s *Service) ListChannels(ctx context.Context) ([]*notifdom.Channel, error) {
	return s.repo.ListChannels(ctx)
}

// DeleteChannel removes a channel.
func (s *Service) DeleteChannel(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.DeleteChannel(ctx, uid)
}

// TestChannel delivers a test message to one channel.
func (s *Service) TestChannel(ctx context.Context, id string) error {
	c, err := s.getChannel(ctx, id)
	if err != nil {
		return err
	}
	err = s.sender.Deliver(ctx, notify.Channel{Type: string(c.Type), Config: c.Config}, notify.Message{
		Title:    "Fleetdock test notification",
		Body:     "This is a test message from Fleetdock. If you can read this, the channel works.",
		Severity: "info",
		Event:    "test",
	})
	if err != nil {
		return apperr.Invalid("channel", "delivery failed: "+err.Error())
	}
	return nil
}

func (s *Service) getChannel(ctx context.Context, id string) (*notifdom.Channel, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.GetChannel(ctx, uid)
}

// ---- Rules ----

// RuleInput describes an alert-rule create/update.
type RuleInput struct {
	Name       string
	TargetType string
	TargetID   string
	Metric     string
	Comparator string
	Threshold  float64
	ForSeconds int
	Severity   string
	ChannelIDs []string
	Enabled    bool
}

func (in RuleInput) parse() (*uuid.UUID, []uuid.UUID, error) {
	var targetID *uuid.UUID
	if in.TargetID != "" {
		id, err := uuid.Parse(in.TargetID)
		if err != nil {
			return nil, nil, apperr.Invalid("target_id", "target_id must be a valid UUID")
		}
		targetID = &id
	}
	channelIDs := make([]uuid.UUID, 0, len(in.ChannelIDs))
	for _, c := range in.ChannelIDs {
		id, err := uuid.Parse(c)
		if err != nil {
			return nil, nil, apperr.Invalid("channel_ids", "channel_ids must be valid UUIDs")
		}
		channelIDs = append(channelIDs, id)
	}
	return targetID, channelIDs, nil
}

// CreateRule validates and persists an alert rule.
func (s *Service) CreateRule(ctx context.Context, in RuleInput) (*notifdom.Rule, error) {
	targetID, channelIDs, err := in.parse()
	if err != nil {
		return nil, err
	}
	r, err := notifdom.NewRule(in.Name, in.TargetType, targetID, notifdom.Metric(in.Metric),
		notifdom.Comparator(in.Comparator), in.Threshold, in.ForSeconds, in.Severity, channelIDs, in.Enabled)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateRule(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// UpdateRule validates and persists rule changes.
func (s *Service) UpdateRule(ctx context.Context, id string, in RuleInput) (*notifdom.Rule, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	r, err := s.repo.GetRule(ctx, uid)
	if err != nil {
		return nil, err
	}
	targetID, channelIDs, err := in.parse()
	if err != nil {
		return nil, err
	}
	if err := r.Apply(in.Name, in.TargetType, targetID, notifdom.Metric(in.Metric),
		notifdom.Comparator(in.Comparator), in.Threshold, in.ForSeconds, in.Severity, channelIDs, in.Enabled); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateRule(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// ListRules returns all alert rules.
func (s *Service) ListRules(ctx context.Context) ([]*notifdom.Rule, error) {
	return s.repo.ListRules(ctx)
}

// DeleteRule removes an alert rule.
func (s *Service) DeleteRule(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.DeleteRule(ctx, uid)
}

// ---- Events + dispatch ----

// Emit queues a system event to all enabled channels.
func (s *Service) Emit(ctx context.Context, eventType, title, message, severity string, aggregateType string, aggregateID uuid.UUID) {
	err := s.repo.Enqueue(ctx, notifdom.Event{
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Title:         title,
		Message:       message,
		Severity:      severity,
	})
	if err != nil {
		slog.Error("notification: enqueue", "event", eventType, "error", err.Error())
	}
}

// DispatchPending delivers unpublished outbox events to their channels and
// reports how many were delivered.
func (s *Service) DispatchPending(ctx context.Context) (int, error) {
	rows, err := s.repo.ListUnpublished(ctx, 50)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, row := range rows {
		channels, err := s.repo.ListEnabledChannels(ctx, row.Event.ChannelIDs)
		if err != nil {
			slog.Error("notification: resolve channels", "error", err.Error())
			_ = s.repo.MarkFailed(ctx, row.ID)
			continue
		}
		ok := true
		for _, ch := range channels {
			derr := s.sender.Deliver(ctx, notify.Channel{Type: string(ch.Type), Config: ch.Config}, notify.Message{
				Title:    row.Event.Title,
				Body:     row.Event.Message,
				Severity: row.Event.Severity,
				Event:    row.Event.EventType,
			})
			if derr != nil {
				slog.Warn("notification: delivery failed", "channel", ch.Name, "error", derr.Error())
				ok = false
			}
		}
		if ok {
			_ = s.repo.MarkPublished(ctx, row.ID)
			delivered++
		} else {
			_ = s.repo.MarkFailed(ctx, row.ID)
		}
	}
	return delivered, nil
}

// ---- Alert evaluation ----

// EvaluateAlerts checks enabled rules against the latest server metrics and
// emits firing/resolved events on state transitions.
func (s *Service) EvaluateAlerts(ctx context.Context) error {
	rules, err := s.repo.ListEnabledRules(ctx)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	samples, err := s.metrics.LatestServerHealth(ctx)
	if err != nil {
		return err
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rule := range rules {
		for i := range samples {
			sample := samples[i]
			if rule.TargetType == "server" && (rule.TargetID == nil || *rule.TargetID != sample.ServerID) {
				continue
			}
			value, ok := metricValue(rule.Metric, sample)
			if !ok {
				continue
			}
			s.evaluateOne(ctx, rule, sample, value, now)
		}
	}
	return nil
}

func (s *Service) evaluateOne(ctx context.Context, rule *notifdom.Rule, sample serverdom.HealthSample, value float64, now time.Time) {
	key := rule.ID.String() + "/" + sample.ServerID.String()
	breaching := rule.Comparator.Compare(value, rule.Threshold)

	if !breaching {
		if s.firing[key] {
			s.Emit(ctx, "alert.resolved",
				fmt.Sprintf("Resolved: %s on %s", rule.Name, sample.ServerName),
				fmt.Sprintf("%s is back to %.1f (threshold %s %.1f)", rule.Metric, value, rule.Comparator, rule.Threshold),
				"info", "alert_rule", rule.ID)
		}
		delete(s.firing, key)
		delete(s.breaching, key)
		return
	}

	since, seen := s.breaching[key]
	if !seen {
		s.breaching[key] = now
		since = now
	}
	if s.firing[key] {
		return // already alerting
	}
	if now.Sub(since) >= time.Duration(rule.ForSeconds)*time.Second {
		s.firing[key] = true
		msg := fmt.Sprintf("%s = %.1f %s threshold %.1f on server %s",
			rule.Metric, value, rule.Comparator, rule.Threshold, sample.ServerName)
		s.emitToChannels(ctx, rule, fmt.Sprintf("Alert: %s", rule.Name), msg)
	}
}

func (s *Service) emitToChannels(ctx context.Context, rule *notifdom.Rule, title, message string) {
	err := s.repo.Enqueue(ctx, notifdom.Event{
		AggregateType: "alert_rule",
		AggregateID:   rule.ID,
		EventType:     "alert.firing",
		Title:         title,
		Message:       message,
		Severity:      rule.Severity,
		ChannelIDs:    rule.ChannelIDs,
	})
	if err != nil {
		slog.Error("notification: enqueue alert", "rule", rule.ID, "error", err.Error())
	}
}

// metricValue extracts a rule metric from a health sample.
func metricValue(m notifdom.Metric, h serverdom.HealthSample) (float64, bool) {
	switch m {
	case notifdom.MetricCPUPct:
		if h.CPUPct == nil {
			return 0, false
		}
		return *h.CPUPct, true
	case notifdom.MetricMemUsedPct:
		return pct(h.MemUsedBytes, h.MemTotalBytes)
	case notifdom.MetricDiskUsedPct:
		return pct(h.DiskUsedBytes, h.DiskTotalBytes)
	case notifdom.MetricConnections:
		if h.ActiveConnections == nil {
			return 0, false
		}
		return float64(*h.ActiveConnections), true
	}
	return 0, false
}

func pct(used, total *int64) (float64, bool) {
	if used == nil || total == nil || *total == 0 {
		return 0, false
	}
	return float64(*used) / float64(*total) * 100, true
}
