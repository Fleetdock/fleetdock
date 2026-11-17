package httpapi

import (
	"testing"

	"github.com/google/uuid"

	notifdom "github.com/Fleetdock/fleetdock/backend/internal/domain/notification"
)

func TestRedactChannelConfigMasksPagerDutyRoutingKey(t *testing.T) {
	c := &notifdom.Channel{
		ID:   uuid.New(),
		Type: notifdom.ChannelPagerDuty,
		Config: map[string]string{
			"routing_key": "secret-routing-key",
			"region":      "us",
		},
	}
	got := redactChannelConfig(c)
	if got["routing_key"] != "••••••" {
		t.Fatalf("routing key was exposed as %q", got["routing_key"])
	}
	if got["region"] != "us" {
		t.Fatalf("non-secret config changed: %v", got)
	}
}

func TestRedactChannelConfigMasksPushoverKeys(t *testing.T) {
	c := &notifdom.Channel{
		ID:     uuid.New(),
		Type:   notifdom.ChannelPushover,
		Config: map[string]string{"app_token": "app-secret", "user_key": "user-secret", "device": "phone"},
	}
	got := redactChannelConfig(c)
	if got["app_token"] != "••••••" || got["user_key"] != "••••••" {
		t.Fatalf("pushover keys were exposed: %v", got)
	}
	if got["device"] != "phone" {
		t.Fatalf("non-secret config changed: %v", got)
	}
}
