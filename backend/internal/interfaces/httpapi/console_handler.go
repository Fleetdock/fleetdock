package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	authzapp "github.com/Fleetdock/fleetdock/backend/internal/app/authz"
	consoleapp "github.com/Fleetdock/fleetdock/backend/internal/app/console"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// ConsoleHandler exposes SQL console history and saved queries. Everything is
// scoped to the calling user.
type ConsoleHandler struct {
	svc      *consoleapp.Service
	resolver *authzapp.Resolver
}

// NewConsoleHandler builds the handler.
func NewConsoleHandler(svc *consoleapp.Service, resolver *authzapp.Resolver) *ConsoleHandler {
	return &ConsoleHandler{svc: svc, resolver: resolver}
}

func requireCaller(r *http.Request) (uuid.UUID, error) {
	if id := callerID(r); id != nil {
		return *id, nil
	}
	return uuid.Nil, apperr.Unauthorized("authentication required")
}

// History handles GET /v1/databases/{id}/query-history.
func (h *ConsoleHandler) History(w http.ResponseWriter, r *http.Request) {
	uid, err := requireCaller(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := h.svc.History(r.Context(), uid, r.PathValue("id"), atoiDefault(r.URL.Query().Get("limit"), 50))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type savedQueryRequest struct {
	Name       string `json:"name"`
	SQL        string `json:"sql"`
	DatabaseID string `json:"database_id"`
}

// checkDatabase requires database:read on a saved query's database, so a
// query cannot be filed under a database the caller cannot see.
func (h *ConsoleHandler) checkDatabase(r *http.Request, databaseID string) error {
	if databaseID == "" {
		return nil
	}
	did, err := uuid.Parse(databaseID)
	if err != nil {
		return apperr.Invalid("database_id", "database_id must be a valid UUID")
	}
	return authorizeResource(r.Context(), h.resolver, "database:read", authz.ResourceDatabase, did)
}

// ListSaved handles GET /v1/saved-queries?database_id=.
func (h *ConsoleHandler) ListSaved(w http.ResponseWriter, r *http.Request) {
	uid, err := requireCaller(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := h.svc.ListSaved(r.Context(), uid, r.URL.Query().Get("database_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// CreateSaved handles POST /v1/saved-queries.
func (h *ConsoleHandler) CreateSaved(w http.ResponseWriter, r *http.Request) {
	uid, err := requireCaller(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var req savedQueryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.checkDatabase(r, req.DatabaseID); err != nil {
		writeError(w, err)
		return
	}
	q, err := h.svc.CreateSaved(r.Context(), uid, consoleapp.SavedInput{Name: req.Name, SQL: req.SQL, DatabaseID: req.DatabaseID})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, q)
}

// UpdateSaved handles PATCH /v1/saved-queries/{id}.
func (h *ConsoleHandler) UpdateSaved(w http.ResponseWriter, r *http.Request) {
	uid, err := requireCaller(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var req savedQueryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	q, err := h.svc.UpdateSaved(r.Context(), uid, r.PathValue("id"), consoleapp.SavedInput{Name: req.Name, SQL: req.SQL})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

// DeleteSaved handles DELETE /v1/saved-queries/{id}.
func (h *ConsoleHandler) DeleteSaved(w http.ResponseWriter, r *http.Request) {
	uid, err := requireCaller(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.DeleteSaved(r.Context(), uid, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
