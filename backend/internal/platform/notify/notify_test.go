package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// local builds a Sender that may reach the test server on loopback (the
// production client refuses loopback).
func local() *Sender { return &Sender{client: http.DefaultClient, pagerDutyURL: pagerDutyEventsURL} }

func TestDeliverWebhookAndSlack(t *testing.T) {
	var got []map[string]string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" || r.Method != http.MethodPost {
			t.Errorf("request %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		b, _ := io.ReadAll(r.Body)
		var m map[string]string
		_ = json.Unmarshal(b, &m)
		got = append(got, m)
		w.WriteHeader(status)
	}))
	defer srv.Close()

	msg := Message{Title: "Backup failed", Body: "orders: disk full", Severity: "critical", Event: "backup.failed"}
	ctx := context.Background()
	if err := local().Deliver(ctx, Channel{Type: "webhook", Config: map[string]string{"url": srv.URL}}, msg); err != nil {
		t.Fatal(err)
	}
	if err := local().Deliver(ctx, Channel{Type: "slack", Config: map[string]string{"webhook_url": srv.URL}}, msg); err != nil {
		t.Fatal(err)
	}
	if got[0]["event"] != "backup.failed" || got[0]["message"] != "orders: disk full" || got[0]["severity"] != "critical" {
		t.Errorf("webhook payload %v", got[0])
	}
	if !strings.Contains(got[1]["text"], "*Backup failed* [critical]") {
		t.Errorf("slack payload %v", got[1])
	}

	status = http.StatusBadGateway
	if err := local().Deliver(ctx, Channel{Type: "webhook", Config: map[string]string{"url": srv.URL}}, msg); err == nil ||
		!strings.Contains(err.Error(), "502") {
		t.Errorf("a 502 must fail delivery, got %v", err)
	}
}

func TestDeliverPagerDutyTriggerAndResolve(t *testing.T) {
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		got = append(got, payload)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	sender := local()
	sender.pagerDutyURL = srv.URL
	channel := Channel{Type: "pagerduty", Config: map[string]string{"routing_key": "integration-key"}}
	trigger := Message{Title: "CPU high", Body: "cpu = 95", Severity: "critical", Event: "alert.firing", DedupKey: "rule/server"}
	if err := sender.Deliver(context.Background(), channel, trigger); err != nil {
		t.Fatal(err)
	}
	resolve := trigger
	resolve.Event = "alert.resolved"
	resolve.Severity = "info"
	if err := sender.Deliver(context.Background(), channel, resolve); err != nil {
		t.Fatal(err)
	}

	if got[0]["routing_key"] != "integration-key" || got[0]["event_action"] != "trigger" || got[0]["dedup_key"] != "rule/server" {
		t.Fatalf("trigger envelope = %#v", got[0])
	}
	payload, ok := got[0]["payload"].(map[string]any)
	if !ok || payload["severity"] != "critical" || payload["source"] != "Fleetdock" {
		t.Fatalf("trigger payload = %#v", got[0]["payload"])
	}
	if got[1]["event_action"] != "resolve" || got[1]["dedup_key"] != got[0]["dedup_key"] || got[1]["payload"] != nil {
		t.Fatalf("resolve envelope = %#v", got[1])
	}
}

func TestDeliverRefusals(t *testing.T) {
	ctx := context.Background()
	for _, c := range []Channel{
		{Type: "webhook", Config: map[string]string{}},
		{Type: "slack", Config: map[string]string{}},
		{Type: "pagerduty", Config: map[string]string{}},
		{Type: "email", Config: map[string]string{"to": "ops@example.com"}}, // no SMTP configured
		{Type: "pager", Config: map[string]string{}},
	} {
		if err := local().Deliver(ctx, c, Message{}); err == nil {
			t.Errorf("%s with %v delivered", c.Type, c.Config)
		}
	}
	// The production client refuses loopback (SSRF guard).
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	if err := New(SMTPConfig{}).Deliver(ctx, Channel{Type: "webhook", Config: map[string]string{"url": srv.URL}}, Message{}); err == nil {
		t.Error("the guarded client reached loopback")
	}
}

func TestEmailHeadersStayOnOneLine(t *testing.T) {
	m := emailMessage("fleetdock@example.com", "ops@example.com",
		Message{Title: "Backup failed: x\r\nBcc: attacker@example.com", Body: "line 1\r\nline 2", Severity: "critical"})
	head, body, _ := strings.Cut(m, "\r\n\r\n")
	if strings.Contains(head, "\r\nBcc:") || strings.Count(head, "\r\n") != 2 {
		t.Errorf("a title started a header:\n%q", head)
	}
	if !strings.Contains(body, "line 1\r\nline 2") {
		t.Errorf("the body lost its lines: %q", body)
	}
}
