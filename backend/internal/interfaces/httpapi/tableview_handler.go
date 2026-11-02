package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	auditapp "github.com/Fleetdock/fleetdock/backend/internal/app/audit"
	authzapp "github.com/Fleetdock/fleetdock/backend/internal/app/authz"
	tableviewapp "github.com/Fleetdock/fleetdock/backend/internal/app/tableview"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
)

// TableViewHandler serves saved data-browser table views. Routes are under
// requireResourcePerm(database:read) on the database.
type TableViewHandler struct {
	svc      *tableviewapp.Service
	resolver *authzapp.Resolver
}

// NewTableViewHandler builds the handler.
func NewTableViewHandler(svc *tableviewapp.Service, resolver *authzapp.Resolver) *TableViewHandler {
	return &TableViewHandler{svc: svc, resolver: resolver}
}

// List handles GET /v1/databases/{id}/table-views?table=.
func (h *TableViewHandler) List(w http.ResponseWriter, r *http.Request) {
	uid, err := requireCaller(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := h.svc.List(r.Context(), r.PathValue("id"), r.URL.Query().Get("table"), uid)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Create handles POST /v1/databases/{id}/table-views.
func (h *TableViewHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid, err := requireCaller(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in tableviewapp.Input
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	v, err := h.svc.Create(r.Context(), r.PathValue("id"), uid, in)
	if err != nil {
		writeError(w, err)
		return
	}
	auditapp.AddMeta(r.Context(), "view", v.Name)
	auditapp.AddMeta(r.Context(), "table", v.Table)
	auditapp.AddMeta(r.Context(), "shared", v.Shared)
	writeJSON(w, http.StatusCreated, v)
}

// Update handles PATCH /v1/databases/{id}/table-views/{viewId}.
func (h *TableViewHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid, err := requireCaller(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in tableviewapp.Input
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	v, err := h.svc.Update(r.Context(), r.PathValue("id"), r.PathValue("viewId"), uid, in)
	if err != nil {
		writeError(w, err)
		return
	}
	auditapp.AddMeta(r.Context(), "view", v.Name)
	auditapp.AddMeta(r.Context(), "table", v.Table)
	auditapp.AddMeta(r.Context(), "shared", v.Shared)
	writeJSON(w, http.StatusOK, v)
}

// Delete handles DELETE /v1/databases/{id}/table-views/{viewId}.
func (h *TableViewHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid, err := requireCaller(r)
	if err != nil {
		writeError(w, err)
		return
	}
	canWrite := false
	if did, err := uuid.Parse(r.PathValue("id")); err == nil {
		canWrite = authorizeResource(r.Context(), h.resolver, "database:write", authz.ResourceDatabase, did) == nil
	}
	if err := h.svc.Delete(r.Context(), r.PathValue("id"), r.PathValue("viewId"), uid, canWrite); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
