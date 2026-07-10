package httpapi

import (
	"net/http"
	"time"

	notificationapp "github.com/TajBrains/fleetdock/backend/internal/app/notification"
	notifdom "github.com/TajBrains/fleetdock/backend/internal/domain/notification"
)

// NotificationHandler exposes channel + alert-rule endpoints.
type NotificationHandler struct {
	svc *notificationapp.Service
}

// NewNotificationHandler builds the notification handler.
func NewNotificationHandler(svc *notificationapp.Service) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// ---- Channels ----

type channelRequest struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	Config  map[string]string `json:"config"`
	Enabled *bool             `json:"enabled"`
}

type channelResponse struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Config    map[string]string `json:"config"`
	Enabled   bool              `json:"enabled"`
	CreatedAt time.Time         `json:"created_at"`
}

func toChannelResponse(c *notifdom.Channel) channelResponse {
	return channelResponse{
		ID:        c.ID.String(),
		Name:      c.Name,
		Type:      string(c.Type),
		Config:    redactChannelConfig(c),
		Enabled:   c.Enabled,
		CreatedAt: c.CreatedAt,
	}
}

// redactChannelConfig masks secret-ish values so they are not echoed back.
func redactChannelConfig(c *notifdom.Channel) map[string]string {
	out := map[string]string{}
	for k, v := range c.Config {
		if (k == "webhook_url" || k == "url" || k == "password") && v != "" {
			out[k] = "••••••"
			continue
		}
		out[k] = v
	}
	return out
}

func channelInput(req channelRequest) notificationapp.ChannelInput {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return notificationapp.ChannelInput{Name: req.Name, Type: req.Type, Config: req.Config, Enabled: enabled}
}

// CreateChannel handles POST /v1/notification-channels.
func (h *NotificationHandler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	var req channelRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	c, err := h.svc.CreateChannel(r.Context(), channelInput(req))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toChannelResponse(c))
}

// ListChannels handles GET /v1/notification-channels.
func (h *NotificationHandler) ListChannels(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListChannels(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]channelResponse, 0, len(items))
	for _, c := range items {
		out = append(out, toChannelResponse(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// UpdateChannel handles PATCH /v1/notification-channels/{id}.
func (h *NotificationHandler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	var req channelRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	c, err := h.svc.UpdateChannel(r.Context(), r.PathValue("id"), channelInput(req))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toChannelResponse(c))
}

// DeleteChannel handles DELETE /v1/notification-channels/{id}.
func (h *NotificationHandler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteChannel(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// TestChannel handles POST /v1/notification-channels/{id}/test.
func (h *NotificationHandler) TestChannel(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.TestChannel(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- Alert rules ----

type ruleRequest struct {
	Name       string   `json:"name"`
	TargetType string   `json:"target_type"`
	TargetID   string   `json:"target_id"`
	Metric     string   `json:"metric"`
	Comparator string   `json:"comparator"`
	Threshold  float64  `json:"threshold"`
	ForSeconds int      `json:"for_seconds"`
	Severity   string   `json:"severity"`
	ChannelIDs []string `json:"channel_ids"`
	Enabled    *bool    `json:"enabled"`
}

type ruleResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	TargetType string    `json:"target_type"`
	TargetID   *string   `json:"target_id,omitempty"`
	Metric     string    `json:"metric"`
	Comparator string    `json:"comparator"`
	Threshold  float64   `json:"threshold"`
	ForSeconds int       `json:"for_seconds"`
	Severity   string    `json:"severity"`
	ChannelIDs []string  `json:"channel_ids"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
}

func toRuleResponse(r *notifdom.Rule) ruleResponse {
	var targetID *string
	if r.TargetID != nil {
		s := r.TargetID.String()
		targetID = &s
	}
	channelIDs := make([]string, 0, len(r.ChannelIDs))
	for _, c := range r.ChannelIDs {
		channelIDs = append(channelIDs, c.String())
	}
	return ruleResponse{
		ID:         r.ID.String(),
		Name:       r.Name,
		TargetType: r.TargetType,
		TargetID:   targetID,
		Metric:     string(r.Metric),
		Comparator: string(r.Comparator),
		Threshold:  r.Threshold,
		ForSeconds: r.ForSeconds,
		Severity:   r.Severity,
		ChannelIDs: channelIDs,
		Enabled:    r.Enabled,
		CreatedAt:  r.CreatedAt,
	}
}

func ruleInput(req ruleRequest) notificationapp.RuleInput {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	ids := req.ChannelIDs
	if ids == nil {
		ids = []string{}
	}
	return notificationapp.RuleInput{
		Name:       req.Name,
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		Metric:     req.Metric,
		Comparator: req.Comparator,
		Threshold:  req.Threshold,
		ForSeconds: req.ForSeconds,
		Severity:   req.Severity,
		ChannelIDs: ids,
		Enabled:    enabled,
	}
}

// CreateRule handles POST /v1/alert-rules.
func (h *NotificationHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var req ruleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	rule, err := h.svc.CreateRule(r.Context(), ruleInput(req))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toRuleResponse(rule))
}

// ListRules handles GET /v1/alert-rules.
func (h *NotificationHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListRules(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]ruleResponse, 0, len(items))
	for _, rule := range items {
		out = append(out, toRuleResponse(rule))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// UpdateRule handles PATCH /v1/alert-rules/{id}.
func (h *NotificationHandler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	var req ruleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	rule, err := h.svc.UpdateRule(r.Context(), r.PathValue("id"), ruleInput(req))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toRuleResponse(rule))
}

// DeleteRule handles DELETE /v1/alert-rules/{id}.
func (h *NotificationHandler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteRule(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
