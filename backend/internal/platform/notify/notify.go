// Package notify delivers notification events to channels (webhook, Slack,
// email, PagerDuty, Pushover). It has no dependency on the domain or storage layers — callers pass
// a plain Message and Channel description.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/netsafe"
)

// SMTPConfig configures outbound email. When Host is empty, email delivery is
// disabled and email channels report a clear error.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// Message is a channel-agnostic notification payload.
type Message struct {
	Title    string
	Body     string
	Severity string
	Event    string
	DedupKey string
}

// Channel is the subset of a notification channel notify needs to deliver.
type Channel struct {
	Type   string
	Config map[string]string
}

// Sender delivers messages to channels.
type Sender struct {
	smtp         SMTPConfig
	client       *http.Client
	pagerDutyURL string
	pushoverURL  string
}

const (
	pagerDutyEventsURL  = "https://events.pagerduty.com/v2/enqueue"
	pushoverMessagesURL = "https://api.pushover.net/1/messages.json"
)

// New builds a Sender. HTTP delivery uses an SSRF-guarded client that refuses
// to connect to loopback, link-local or metadata addresses.
func New(smtp SMTPConfig) *Sender {
	return &Sender{
		smtp:         smtp,
		client:       netsafe.GuardedClient(10 * time.Second),
		pagerDutyURL: pagerDutyEventsURL,
		pushoverURL:  pushoverMessagesURL,
	}
}

// Deliver sends a message to a single channel.
func (s *Sender) Deliver(ctx context.Context, ch Channel, msg Message) error {
	switch ch.Type {
	case "webhook":
		return s.deliverWebhook(ctx, ch.Config["url"], msg)
	case "slack":
		return s.deliverSlack(ctx, ch.Config["webhook_url"], msg)
	case "email":
		return s.deliverEmail(ch.Config["to"], msg)
	case "pagerduty":
		return s.deliverPagerDuty(ctx, ch.Config["routing_key"], msg)
	case "pushover":
		return s.deliverPushover(ctx, ch.Config, msg)
	default:
		return fmt.Errorf("unknown channel type %q", ch.Type)
	}
}

func (s *Sender) deliverPagerDuty(ctx context.Context, routingKey string, msg Message) error {
	if routingKey == "" {
		return fmt.Errorf("pagerduty routing key is empty")
	}
	if msg.DedupKey == "" {
		return fmt.Errorf("pagerduty deduplication key is empty")
	}
	if msg.Severity == "info" && msg.Event != "alert.resolved" && msg.Event != "test" {
		return nil
	}
	action := "trigger"
	payload := map[string]any{
		"routing_key":  routingKey,
		"event_action": action,
		"dedup_key":    msg.DedupKey,
	}
	if msg.Event == "alert.resolved" {
		payload["event_action"] = "resolve"
	} else {
		severity := msg.Severity
		switch severity {
		case "info", "warning", "critical":
		default:
			severity = "info"
		}
		payload["payload"] = map[string]any{
			"summary":        msg.Title,
			"source":         "Fleetdock",
			"severity":       severity,
			"custom_details": map[string]string{"message": msg.Body, "event": msg.Event},
		}
	}
	body, _ := json.Marshal(payload)
	return s.post(ctx, s.pagerDutyURL, body)
}

// Pushover's limits on a message's title and text, in characters.
const (
	pushoverTitleMax   = 250
	pushoverMessageMax = 1024
)

// pushoverPriority maps a severity to a Pushover priority: critical alerts
// bypass the recipient's quiet hours, informational ones (recoveries among
// them) arrive without a sound. Emergency priority (2) is not used: it repeats
// until acknowledged, which Fleetdock has no way to stop on recovery.
func pushoverPriority(msg Message) int {
	switch {
	case msg.Event == "test":
		return 0
	case msg.Severity == "critical":
		return 1
	case msg.Severity == "warning":
		return 0
	default:
		return -1
	}
}

func (s *Sender) deliverPushover(ctx context.Context, config map[string]string, msg Message) error {
	if config["app_token"] == "" {
		return fmt.Errorf("pushover application token is empty")
	}
	if config["user_key"] == "" {
		return fmt.Errorf("pushover user key is empty")
	}
	text := msg.Body
	if text == "" {
		text = msg.Title // Pushover rejects an empty message
	}
	payload := map[string]any{
		"token":    config["app_token"],
		"user":     config["user_key"],
		"title":    truncateRunes(msg.Title, pushoverTitleMax),
		"message":  truncateRunes(text, pushoverMessageMax),
		"priority": pushoverPriority(msg),
	}
	if d := config["device"]; d != "" {
		payload["device"] = d
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.pushoverURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 {
		return nil
	}
	// A rejected request names what is wrong ("application token is
	// invalid", "user key is invalid"); pass that on so a test shows it.
	var reply struct {
		Errors []string `json:"errors"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&reply) == nil && len(reply.Errors) > 0 {
		return fmt.Errorf("pushover returned status %d: %s", resp.StatusCode, strings.Join(reply.Errors, "; "))
	}
	return fmt.Errorf("delivery returned status %d", resp.StatusCode)
}

// truncateRunes shortens s to at most n characters, ending in an ellipsis
// when cut.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func (s *Sender) deliverWebhook(ctx context.Context, url string, msg Message) error {
	if url == "" {
		return fmt.Errorf("webhook url is empty")
	}
	body, _ := json.Marshal(map[string]string{
		"event":    msg.Event,
		"title":    msg.Title,
		"message":  msg.Body,
		"severity": msg.Severity,
	})
	return s.post(ctx, url, body)
}

func (s *Sender) deliverSlack(ctx context.Context, url string, msg Message) error {
	if url == "" {
		return fmt.Errorf("slack webhook url is empty")
	}
	text := fmt.Sprintf("*%s* [%s]\n%s", msg.Title, msg.Severity, msg.Body)
	body, _ := json.Marshal(map[string]string{"text": text})
	return s.post(ctx, url, body)
}

func (s *Sender) post(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("delivery returned status %d", resp.StatusCode)
	}
	return nil
}

// headerSafe keeps a value on one header line. Titles carry user-chosen
// names (a PostgreSQL database may be called "x\r\nBcc: ..."), which must not
// start new headers.
var headerSafe = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")

func emailMessage(from, to string, msg Message) string {
	return fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: [%s] %s\r\n\r\n%s\r\n",
		headerSafe.Replace(from), headerSafe.Replace(to), headerSafe.Replace(msg.Severity),
		headerSafe.Replace(msg.Title), msg.Body)
}

func (s *Sender) deliverEmail(to string, msg Message) error {
	if s.smtp.Host == "" {
		return fmt.Errorf("email delivery is not configured (set FLEETDOCK_SMTP_HOST)")
	}
	if to == "" {
		return fmt.Errorf("email channel has no recipient")
	}
	addr := s.smtp.Host + ":" + s.smtp.Port
	body := emailMessage(s.smtp.From, to, msg)
	var auth smtp.Auth
	if s.smtp.Username != "" {
		auth = smtp.PlainAuth("", s.smtp.Username, s.smtp.Password, s.smtp.Host)
	}
	return smtp.SendMail(addr, auth, s.smtp.From, []string{to}, []byte(body))
}
