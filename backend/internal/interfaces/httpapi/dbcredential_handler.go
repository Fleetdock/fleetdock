package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	dbcredentialapp "github.com/Fleetdock/fleetdock/backend/internal/app/dbcredential"
)

// DBCredentialHandler exposes application credential endpoints.
type DBCredentialHandler struct {
	svc *dbcredentialapp.Service
}

// NewDBCredentialHandler builds the handler.
func NewDBCredentialHandler(svc *dbcredentialapp.Service) *DBCredentialHandler {
	return &DBCredentialHandler{svc: svc}
}

type createCredentialRequest struct {
	Name        string     `json:"name"`
	AccessLevel string     `json:"access_level"`
	Username    string     `json:"username"`
	AccountHost string     `json:"account_host"`
	ExpiresAt   *time.Time `json:"expires_at"`
	UsePublic   bool       `json:"use_public"`
}

// List handles GET /v1/databases/{id}/credentials.
func (h *DBCredentialHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Create handles POST /v1/databases/{id}/credentials.
func (h *DBCredentialHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createCredentialRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	var actor *uuid.UUID
	if p := principalFrom(r.Context()); p != nil {
		actor = &p.UserID
	}
	out, err := h.svc.Create(r.Context(), r.PathValue("id"), dbcredentialapp.CreateInput{
		Name:        req.Name,
		AccessLevel: req.AccessLevel,
		Username:    req.Username,
		AccountHost: req.AccountHost,
		ExpiresAt:   req.ExpiresAt,
		UsePublic:   req.UsePublic,
	}, actor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// Rotate handles POST /v1/databases/{id}/credentials/{credentialId}/rotate.
func (h *DBCredentialHandler) Rotate(w http.ResponseWriter, r *http.Request) {
	var actor *uuid.UUID
	if p := principalFrom(r.Context()); p != nil {
		actor = &p.UserID
	}
	out, err := h.svc.Rotate(r.Context(), r.PathValue("id"), r.PathValue("credentialId"), actor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Revoke handles DELETE /v1/databases/{id}/credentials/{credentialId}.
func (h *DBCredentialHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	var actor *uuid.UUID
	if p := principalFrom(r.Context()); p != nil {
		actor = &p.UserID
	}
	if err := h.svc.Revoke(r.Context(), r.PathValue("id"), r.PathValue("credentialId"), actor); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
