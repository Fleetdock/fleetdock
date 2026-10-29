package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	backupcopyapp "github.com/Fleetdock/fleetdock/backend/internal/app/backupcopy"
	dbadminapp "github.com/Fleetdock/fleetdock/backend/internal/app/dbadmin"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// Backup copies ("compare with a backup"): the routes live on the DB admin
// handler to share its data-access checks.

// SetCopies enables the backup-copy routes.
func (h *DBAdminHandler) SetCopies(c *backupcopyapp.Service) { h.copies = c }

func pathUUID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil, apperr.Invalid(name, "must be a valid UUID")
	}
	return id, nil
}

// ListCopies handles GET /v1/databases/{id}/backup-copies.
func (h *DBAdminHandler) ListCopies(w http.ResponseWriter, r *http.Request) {
	did, err := pathUUID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := h.copies.List(r.Context(), did)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// CreateCopy handles POST /v1/databases/{id}/backup-copies. A copy holds the
// database's data, so opening one also needs data access.
func (h *DBAdminHandler) CreateCopy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BackupID string `json:"backup_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
	did, err := pathUUID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	bid, err := uuid.Parse(req.BackupID)
	if err != nil {
		writeError(w, apperr.Invalid("backup_id", "must be a valid UUID"))
		return
	}
	c, err := h.copies.Create(r.Context(), did, bid, callerID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, c)
}

// GetCopy handles GET /v1/databases/{id}/backup-copies/{copyId}.
func (h *DBAdminHandler) GetCopy(w http.ResponseWriter, r *http.Request) {
	did, err := pathUUID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	cid, err := pathUUID(r, "copyId")
	if err != nil {
		writeError(w, err)
		return
	}
	c, err := h.copies.Get(r.Context(), did, cid)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// ExtendCopy handles POST /v1/databases/{id}/backup-copies/{copyId}/extend.
func (h *DBAdminHandler) ExtendCopy(w http.ResponseWriter, r *http.Request) {
	did, err := pathUUID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	cid, err := pathUUID(r, "copyId")
	if err != nil {
		writeError(w, err)
		return
	}
	c, err := h.copies.Extend(r.Context(), did, cid)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// DiscardCopy handles DELETE /v1/databases/{id}/backup-copies/{copyId}.
func (h *DBAdminHandler) DiscardCopy(w http.ResponseWriter, r *http.Request) {
	did, err := pathUUID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	cid, err := pathUUID(r, "copyId")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.copies.Discard(r.Context(), did, cid, callerID(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DiffCopy handles POST /v1/databases/{id}/backup-copies/{copyId}/diff.
func (h *DBAdminHandler) DiffCopy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Table  string                 `json:"table"`
		Cursor *dbadminapp.DiffCursor `json:"cursor"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
	cid, err := pathUUID(r, "copyId")
	if err != nil {
		writeError(w, err)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(90 * time.Second))
	out, err := h.svc.DiffCopy(r.Context(), r.PathValue("id"), cid, req.Table, req.Cursor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// RestoreCopyRows handles POST /v1/databases/{id}/backup-copies/{copyId}/restore-rows.
func (h *DBAdminHandler) RestoreCopyRows(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Table string                       `json:"table"`
		Rows  []dbadminapp.RestoreRowInput `json:"rows"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.editable(r); err != nil {
		writeError(w, err)
		return
	}
	cid, err := pathUUID(r, "copyId")
	if err != nil {
		writeError(w, err)
		return
	}
	meta, err := editMeta(r, "")
	if err != nil {
		writeError(w, err)
		return
	}
	batch := uuid.New()
	meta.BatchID = &batch
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * time.Minute))
	items, err := h.svc.RestoreRows(r.Context(), r.PathValue("id"), cid, req.Table, req.Rows, meta)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "batch_id": batch})
}
