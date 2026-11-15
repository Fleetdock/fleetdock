package notificationapp

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	notifdom "github.com/Fleetdock/fleetdock/backend/internal/domain/notification"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

type memChannels struct {
	notifdom.Repository
	items map[uuid.UUID]*notifdom.Channel
}

func clone(c *notifdom.Channel) *notifdom.Channel {
	cp := *c
	cp.Config = map[string]string{}
	for k, v := range c.Config {
		cp.Config[k] = v
	}
	return &cp
}

func (r *memChannels) CreateChannel(_ context.Context, c *notifdom.Channel) error {
	r.items[c.ID] = clone(c)
	return nil
}
func (r *memChannels) UpdateChannel(_ context.Context, c *notifdom.Channel) error {
	r.items[c.ID] = clone(c)
	return nil
}
func (r *memChannels) GetChannel(_ context.Context, id uuid.UUID) (*notifdom.Channel, error) {
	c, ok := r.items[id]
	if !ok {
		return nil, apperr.NotFound("channel not found")
	}
	return clone(c), nil
}
func (r *memChannels) ListChannels(context.Context) ([]*notifdom.Channel, error) {
	var out []*notifdom.Channel
	for _, c := range r.items {
		out = append(out, clone(c))
	}
	return out, nil
}
func (r *memChannels) DeleteChannel(_ context.Context, id uuid.UUID) error {
	delete(r.items, id)
	return nil
}

type memSecrets map[string]string

func (m memSecrets) Put(_ context.Context, ref string, _ secretdom.Kind, p []byte) error {
	m[ref] = string(p)
	return nil
}
func (m memSecrets) Get(_ context.Context, ref string) ([]byte, error) {
	v, ok := m[ref]
	if !ok {
		return nil, apperr.NotFound("secret not found")
	}
	return []byte(v), nil
}
func (m memSecrets) Delete(_ context.Context, ref string) error {
	delete(m, ref)
	return nil
}

func TestChannelSecretsAreSealed(t *testing.T) {
	ctx := context.Background()
	repo := &memChannels{items: map[uuid.UUID]*notifdom.Channel{}}
	store := memSecrets{"instance/root/other": "db-root-password"}
	svc := NewService(repo, nil, nil).WithSecrets(store)

	c, err := svc.CreateChannel(ctx, ChannelInput{Name: "ops", Type: "webhook",
		Config: map[string]string{"url": "https://hooks.example.com/T0/secret-token"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	stored := repo.items[c.ID].Config["url"]
	if strings.Contains(stored, "secret-token") || stored != sealedValue(c.ID, "url") {
		t.Fatalf("the URL is stored in the channel: %q", stored)
	}
	open, err := svc.open(ctx, repo.items[c.ID])
	if err != nil || open["url"] != "https://hooks.example.com/T0/secret-token" {
		t.Fatalf("open = %v, %v", open, err)
	}

	// Saving the masked value keeps the secret; a new value replaces it.
	if _, err := svc.UpdateChannel(ctx, c.ID.String(), ChannelInput{Name: "ops", Type: "webhook",
		Config: map[string]string{"url": RedactedMask}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if open, _ := svc.open(ctx, repo.items[c.ID]); open["url"] != "https://hooks.example.com/T0/secret-token" {
		t.Fatalf("masked update lost the URL: %v", open)
	}
	if _, err := svc.UpdateChannel(ctx, c.ID.String(), ChannelInput{Name: "ops", Type: "webhook",
		Config: map[string]string{"url": "https://hooks.example.com/new"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if open, _ := svc.open(ctx, repo.items[c.ID]); open["url"] != "https://hooks.example.com/new" {
		t.Fatalf("new URL not stored: %v", open)
	}

	// A config value naming someone else's secret is plain text: it is
	// sealed as typed and never resolved to that secret.
	e, err := svc.CreateChannel(ctx, ChannelInput{Name: "mail", Type: "email",
		Config: map[string]string{"to": "ops@example.com", "password": "secret:instance/root/other"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if open, _ := svc.open(ctx, repo.items[e.ID]); open["password"] != "secret:instance/root/other" {
		t.Fatalf("another secret was resolved: %q", open["password"])
	}

	if err := svc.DeleteChannel(ctx, c.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, ok := store[channelSecretRef(c.ID, "url")]; ok {
		t.Fatal("the deleted channel's secret is still stored")
	}
}

func TestPagerDutyRoutingKeyIsSealed(t *testing.T) {
	ctx := context.Background()
	repo := &memChannels{items: map[uuid.UUID]*notifdom.Channel{}}
	store := memSecrets{}
	svc := NewService(repo, nil, nil).WithSecrets(store)

	c, err := svc.CreateChannel(ctx, ChannelInput{Name: "on-call", Type: "pagerduty",
		Config: map[string]string{"routing_key": "pd-secret"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := repo.items[c.ID].Config["routing_key"]; got != sealedValue(c.ID, "routing_key") {
		t.Fatalf("routing key stored as %q", got)
	}
	if open, _ := svc.open(ctx, repo.items[c.ID]); open["routing_key"] != "pd-secret" {
		t.Fatalf("opened config = %v", open)
	}
}

func TestSealExisting(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	repo := &memChannels{items: map[uuid.UUID]*notifdom.Channel{id: {
		ID: id, Name: "slack", Type: notifdom.ChannelSlack,
		Config: map[string]string{"webhook_url": "https://hooks.slack.com/services/X", "channel": "#ops"},
	}}}
	store := memSecrets{}
	svc := NewService(repo, nil, nil).WithSecrets(store)

	for run, want := range []int{1, 0} {
		n, err := svc.SealExisting(ctx)
		if err != nil || n != want {
			t.Fatalf("run %d: sealed %d, %v; want %d", run, n, err, want)
		}
	}
	got := repo.items[id].Config
	if got["webhook_url"] != sealedValue(id, "webhook_url") || got["channel"] != "#ops" {
		t.Fatalf("config after sealing: %v", got)
	}
	if store[channelSecretRef(id, "webhook_url")] != "https://hooks.slack.com/services/X" {
		t.Fatalf("store = %v", store)
	}
}
