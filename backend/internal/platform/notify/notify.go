// Package notify delivers notification events to channels (webhook, Slack,
// email). It has no dependency on the domain or storage layers — callers pass
// a plain Message and Channel description.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"time"
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
}

// Channel is the subset of a notification channel notify needs to deliver.
type Channel struct {
	Type   string
	Config map[string]string
}

// Sender delivers messages to channels.
type Sender struct {
	smtp   SMTPConfig
	client *http.Client
}

// New builds a Sender.
func New(smtp SMTPConfig) *Sender {
	return &Sender{smtp: smtp, client: &http.Client{Timeout: 10 * time.Second}}
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
	default:
		return fmt.Errorf("unknown channel type %q", ch.Type)
	}
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

func (s *Sender) deliverEmail(to string, msg Message) error {
	if s.smtp.Host == "" {
		return fmt.Errorf("email delivery is not configured (set MDCP_SMTP_HOST)")
	}
	if to == "" {
		return fmt.Errorf("email channel has no recipient")
	}
	addr := s.smtp.Host + ":" + s.smtp.Port
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: [%s] %s\r\n\r\n%s\r\n",
		s.smtp.From, to, msg.Severity, msg.Title, msg.Body)
	var auth smtp.Auth
	if s.smtp.Username != "" {
		auth = smtp.PlainAuth("", s.smtp.Username, s.smtp.Password, s.smtp.Host)
	}
	return smtp.SendMail(addr, auth, s.smtp.From, []string{to}, []byte(body))
}
