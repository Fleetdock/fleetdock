package notificationapp

import (
	"context"
	"testing"

	"github.com/google/uuid"

	notifdom "github.com/Fleetdock/fleetdock/backend/internal/domain/notification"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
)

type alertRepo struct {
	notifdom.Repository
	rules  []*notifdom.Rule
	events []notifdom.Event
}

func (r *alertRepo) ListEnabledRules(context.Context) ([]*notifdom.Rule, error) { return r.rules, nil }
func (r *alertRepo) Enqueue(_ context.Context, event notifdom.Event) error {
	r.events = append(r.events, event)
	return nil
}

type alertMetrics struct{ samples []serverdom.HealthSample }

func (m *alertMetrics) LatestServerHealth(context.Context) ([]serverdom.HealthSample, error) {
	return m.samples, nil
}

func TestAlertRecoveryKeepsChannelsAndDedupKey(t *testing.T) {
	serverID, channelID := uuid.New(), uuid.New()
	rule, err := notifdom.NewRule("CPU", "server", &serverID, notifdom.MetricCPUPct,
		notifdom.CmpGt, 80, 0, "critical", []uuid.UUID{channelID}, true)
	if err != nil {
		t.Fatal(err)
	}
	cpu := 95.0
	metrics := &alertMetrics{samples: []serverdom.HealthSample{{ServerID: serverID, ServerName: "db-1", CPUPct: &cpu}}}
	repo := &alertRepo{rules: []*notifdom.Rule{rule}}
	svc := NewService(repo, nil, metrics)

	if err := svc.EvaluateAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	cpu = 20
	if err := svc.EvaluateAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(repo.events) != 2 {
		t.Fatalf("events = %d, want firing and resolved", len(repo.events))
	}
	firing, resolved := repo.events[0], repo.events[1]
	if firing.EventType != "alert.firing" || resolved.EventType != "alert.resolved" {
		t.Fatalf("event types = %q, %q", firing.EventType, resolved.EventType)
	}
	if firing.DedupKey == "" || resolved.DedupKey != firing.DedupKey {
		t.Fatalf("dedup keys = %q, %q", firing.DedupKey, resolved.DedupKey)
	}
	if len(resolved.ChannelIDs) != 1 || resolved.ChannelIDs[0] != channelID {
		t.Fatalf("resolved channels = %v", resolved.ChannelIDs)
	}
}
