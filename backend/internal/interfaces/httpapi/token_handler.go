package httpapi

import (
	"net/http"
	"time"

	tokenapp "github.com/Fleetdock/fleetdock/backend/internal/app/token"
	tokendom "github.com/Fleetdock/fleetdock/backend/internal/domain/token"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// TokenHandler exposes API-token endpoints scoped to the caller.
type TokenHandler struct {
	svc *tokenapp.Service
}

// NewTokenHandler builds the token handler.
func NewTokenHandler(svc *tokenapp.Service) *TokenHandler { return &TokenHandler{svc: svc} }

type createTokenRequest struct {
	Name     string   `json:"name"`
	Scopes   []string `json:"scopes"`
	TTLHours *int     `json:"ttl_hours"` // optional token lifetime in hours
}

type tokenResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type createTokenResponse struct {
	tokenResponse
	Token string `json:"token"` // plaintext secret, shown once
}

func toTokenResponse(t *tokendom.Token) tokenResponse {
	return tokenResponse{
		ID:         t.ID.String(),
		Name:       t.Name,
		Prefix:     t.Prefix,
		Scopes:     t.Scopes,
		LastUsedAt: t.LastUsedAt,
		ExpiresAt:  t.ExpiresAt,
		RevokedAt:  t.RevokedAt,
		CreatedAt:  t.CreatedAt,
	}
}

// Create handles POST /v1/tokens.
func (h *TokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	p := principalFrom(r.Context())
	var expiresAt *time.Time
	if req.TTLHours != nil {
		if *req.TTLHours <= 0 {
			writeError(w, apperr.Invalid("ttl_hours", "ttl_hours must be positive"))
			return
		}
		t := time.Now().Add(time.Duration(*req.TTLHours) * time.Hour)
		expiresAt = &t
	}
	created, err := h.svc.Create(r.Context(), tokenapp.CreateInput{
		UserID:        p.UserID,
		Name:          req.Name,
		Scopes:        req.Scopes,
		AllowedScopes: p.AllPermissions(),
		ExpiresAt:     expiresAt,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, createTokenResponse{
		tokenResponse: toTokenResponse(created.Token),
		Token:         created.Plaintext,
	})
}

// List handles GET /v1/tokens.
func (h *TokenHandler) List(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	tokens, err := h.svc.List(r.Context(), p.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]tokenResponse, 0, len(tokens))
	for _, t := range tokens {
		items = append(items, toTokenResponse(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Revoke handles DELETE /v1/tokens/{id}.
func (h *TokenHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	if err := h.svc.Revoke(r.Context(), p.UserID, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
