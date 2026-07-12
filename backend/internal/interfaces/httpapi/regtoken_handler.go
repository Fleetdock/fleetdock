package httpapi

import (
	"fmt"
	"net/http"
	"time"

	agentapp "github.com/Fleetdock/fleetdock/backend/internal/app/agent"
	regtokendom "github.com/Fleetdock/fleetdock/backend/internal/domain/regtoken"
)

// RegTokenHandler manages agent registration tokens (server connect flow).
type RegTokenHandler struct {
	svc       *agentapp.Service
	publicURL string
}

// NewRegTokenHandler builds the registration-token handler.
func NewRegTokenHandler(svc *agentapp.Service, publicURL string) *RegTokenHandler {
	return &RegTokenHandler{svc: svc, publicURL: publicURL}
}

type createRegTokenRequest struct {
	Name     string `json:"name"`
	TTLHours int    `json:"ttl_hours"`
}

type regTokenResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	ServerID  *string    `json:"server_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func toRegTokenResponse(t *regtokendom.Token) regTokenResponse {
	var serverID *string
	if t.ServerID != nil {
		s := t.ServerID.String()
		serverID = &s
	}
	return regTokenResponse{
		ID:        t.ID.String(),
		Name:      t.Name,
		ExpiresAt: t.ExpiresAt,
		UsedAt:    t.UsedAt,
		ServerID:  serverID,
		CreatedAt: t.CreatedAt,
	}
}

// Create handles POST /v1/agent-tokens: returns the raw token and the
// ready-to-paste install command exactly once.
func (h *RegTokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRegTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	t, raw, err := h.svc.CreateToken(r.Context(), agentapp.CreateTokenInput{
		Name:      req.Name,
		TTL:       time.Duration(req.TTLHours) * time.Hour,
		CreatedBy: callerID(r),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	resp := struct {
		regTokenResponse
		Token          string `json:"token"`
		InstallCommand string `json:"install_command"`
	}{
		regTokenResponse: toRegTokenResponse(t),
		Token:            raw,
		InstallCommand: fmt.Sprintf(
			"curl -sSL %s/install.sh | sudo env FLEETDOCK_URL=%s FLEETDOCK_TOKEN=%s sh",
			h.publicURL, h.publicURL, raw),
	}
	writeJSON(w, http.StatusCreated, resp)
}

// List handles GET /v1/agent-tokens.
func (h *RegTokenHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListTokens(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]regTokenResponse, 0, len(items))
	for _, t := range items {
		out = append(out, toRegTokenResponse(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// Revoke handles DELETE /v1/agent-tokens/{id}.
func (h *RegTokenHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RevokeToken(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
