package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	schemahistoryapp "github.com/Fleetdock/fleetdock/backend/internal/app/schemahistory"
	"github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	schemahistorydom "github.com/Fleetdock/fleetdock/backend/internal/domain/schemahistory"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// Schema history and comparison: the routes live on the DB admin handler to
// share its data-access checks.

// SetSchemaHistory enables the schema-history routes.
func (h *DBAdminHandler) SetSchemaHistory(s *schemahistoryapp.Service) { h.schema = s }

// SchemaSnapshots handles GET /v1/databases/{id}/schema/snapshots.
func (h *DBAdminHandler) SchemaSnapshots(w http.ResponseWriter, r *http.Request) {
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
	did, err := pathUUID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := h.schema.List(r.Context(), did)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// TakeSchemaSnapshot handles POST /v1/databases/{id}/schema/snapshots.
func (h *DBAdminHandler) TakeSchemaSnapshot(w http.ResponseWriter, r *http.Request) {
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
	did, err := pathUUID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	snap, created, err := h.schema.TakeSnapshot(r.Context(), did, schemahistorydom.TriggerManual, callerID(r))
	if err != nil {
		writeError(w, apperr.FromEngine(err, "database"))
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"snapshot": snap, "created": created})
}

// SchemaSnapshot handles GET /v1/databases/{id}/schema/snapshots/{snapshotId}.
func (h *DBAdminHandler) SchemaSnapshot(w http.ResponseWriter, r *http.Request) {
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
	did, err := pathUUID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	sid, err := pathUUID(r, "snapshotId")
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := h.schema.Get(r.Context(), did, sid)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type schemaWatchRequest struct {
	Enabled bool `json:"enabled"`
}

// SetSchemaWatch handles PUT /v1/databases/{id}/schema/watch.
func (h *DBAdminHandler) SetSchemaWatch(w http.ResponseWriter, r *http.Request) {
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
	did, err := pathUUID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var req schemaWatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	out, err := h.schema.SetWatch(r.Context(), did, req.Enabled, callerID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type schemaCompareRequest struct {
	SourceDatabaseID string  `json:"source_database_id"`
	SourceSnapshotID *string `json:"source_snapshot_id"`
	TargetDatabaseID string  `json:"target_database_id"`
}

// CompareSchemas handles POST /v1/schema/compare. Both databases need
// database:read and the same data-access checks as browsing them.
func (h *DBAdminHandler) CompareSchemas(w http.ResponseWriter, r *http.Request) {
	var req schemaCompareRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	in := schemahistoryapp.CompareInput{}
	var err error
	if in.SourceDatabaseID, err = uuid.Parse(req.SourceDatabaseID); err != nil {
		writeError(w, apperr.Invalid("source_database_id", "must be a valid UUID"))
		return
	}
	if in.TargetDatabaseID, err = uuid.Parse(req.TargetDatabaseID); err != nil {
		writeError(w, apperr.Invalid("target_database_id", "must be a valid UUID"))
		return
	}
	if req.SourceSnapshotID != nil && *req.SourceSnapshotID != "" {
		id, err := uuid.Parse(*req.SourceSnapshotID)
		if err != nil {
			writeError(w, apperr.Invalid("source_snapshot_id", "must be a valid UUID"))
			return
		}
		in.SourceSnapshotID = &id
	}
	for _, id := range []uuid.UUID{in.SourceDatabaseID, in.TargetDatabaseID} {
		if err := authorizeResource(r.Context(), h.resolver, "database:read", authz.ResourceDatabase, id); err != nil {
			writeError(w, err)
			return
		}
		if err := authorizeDataAccess(r.Context(), h.resolver, h.svc, id); err != nil {
			writeError(w, err)
			return
		}
	}
	out, err := h.schema.Compare(r.Context(), in)
	if err != nil {
		writeError(w, apperr.FromEngine(err, "database"))
		return
	}
	writeJSON(w, http.StatusOK, out)
}
