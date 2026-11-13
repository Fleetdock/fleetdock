package notificationapp

import (
	"context"
	"testing"

	"github.com/google/uuid"

	notifdom "github.com/Fleetdock/fleetdock/backend/internal/domain/notification"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/notify"
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

type testChannelRepo struct {
	notifdom.Repository
	channel *notifdom.Channel
}

func (r *testChannelRepo) GetChannel(context.Context, uuid.UUID) (*notifdom.Channel, error) {
	return r.channel, nil
}

type recordedDelivery struct{ messages []notify.Message }

func (s *recordedDelivery) Deliver(_ context.Context, _ notify.Channel, msg notify.Message) error {
	s.messages = append(s.messages, msg)
	return nil
}

func TestPagerDutyChannelTestResolvesIncident(t *testing.T) {
	channel := &notifdom.Channel{
		ID: uuid.New(), Type: notifdom.ChannelPagerDuty,
		Config: map[string]string{"routing_key": "test-key"},
	}
	sender := &recordedDelivery{}
	svc := NewService(&testChannelRepo{channel: channel}, nil, nil)
	svc.sender = sender

	if err := svc.TestChannel(context.Background(), channel.ID.String()); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != 2 {
		t.Fatalf("test sent %d messages, want trigger and resolve", len(sender.messages))
	}
	trigger, resolve := sender.messages[0], sender.messages[1]
	if trigger.Event != "test" || resolve.Event != "alert.resolved" || trigger.DedupKey == "" || resolve.DedupKey != trigger.DedupKey {
		t.Fatalf("test event pair has mismatched actions or dedup keys: %#v %#v", trigger, resolve)
	}
}

func TestEmitDeduplicatesByEventAndAggregate(t *testing.T) {
	repo := &alertRepo{}
	svc := NewService(repo, nil, nil)
	backupID, serverID := uuid.New(), uuid.New()

	svc.Emit(context.Background(), "backup.failed", "Backup failed", "", "critical", "backup", backupID)
	svc.Emit(context.Background(), "server.offline", "Server offline", "", "warning", "server", serverID)
	svc.Emit(context.Background(), "backup.failed", "Backup failed", "", "critical", "backup", backupID)

	if len(repo.events) != 3 || repo.events[0].DedupKey == "" || repo.events[0].DedupKey == repo.events[1].DedupKey || repo.events[0].DedupKey != repo.events[2].DedupKey {
		t.Fatalf("event dedup keys = %#v", []string{repo.events[0].DedupKey, repo.events[1].DedupKey, repo.events[2].DedupKey})
	}
}

type dispatchRepo struct {
	notifdom.Repository
	rows      []notifdom.OutboxRow
	channels  []*notifdom.Channel
	published []int64
}

func (r *dispatchRepo) ListUnpublished(context.Context, int) ([]notifdom.OutboxRow, error) {
	return r.rows, nil
}

func (r *dispatchRepo) ListEnabledChannels(context.Context, []uuid.UUID) ([]*notifdom.Channel, error) {
	return r.channels, nil
}

func (r *dispatchRepo) MarkPublished(_ context.Context, id int64) error {
	r.published = append(r.published, id)
	return nil
}

func (*dispatchRepo) MarkFailed(context.Context, int64) error { return nil }

func TestDispatchPendingGivesLegacyEventsUniqueDedupKeys(t *testing.T) {
	repo := &dispatchRepo{
		rows: []notifdom.OutboxRow{{ID: 42, Event: notifdom.Event{EventType: "backup.failed"}}},
		channels: []*notifdom.Channel{{
			Type: notifdom.ChannelPagerDuty, Config: map[string]string{"routing_key": "test-key"},
		}},
	}
	sender := &recordedDelivery{}
	svc := NewService(repo, nil, nil)
	svc.sender = sender

	delivered, err := svc.DispatchPending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 1 || len(sender.messages) != 1 || sender.messages[0].DedupKey != "fleetdock/outbox/42" {
		t.Fatalf("delivered = %d, messages = %#v", delivered, sender.messages)
	}
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
