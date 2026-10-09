package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	authzapp "github.com/Fleetdock/fleetdock/backend/internal/app/authz"
	backupapp "github.com/Fleetdock/fleetdock/backend/internal/app/backup"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	backupdom "github.com/Fleetdock/fleetdock/backend/internal/domain/backup"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// BackupHandler exposes backup + restore endpoints.
type BackupHandler struct {
	svc      *backupapp.Service
	resolver *authzapp.Resolver
	access   dataAccessLookup
}

// WithDataAccess makes SQL imports follow the instance's data access rules:
// an import runs as the data login, so where that is the admin login only
// instance administrators may import.
func (h *BackupHandler) WithDataAccess(l dataAccessLookup) *BackupHandler {
	h.access = l
	return h
}

// NewBackupHandler builds the backup handler.
func NewBackupHandler(svc *backupapp.Service, resolver *authzapp.Resolver) *BackupHandler {
	return &BackupHandler{svc: svc, resolver: resolver}
}

type triggerBackupRequest struct {
	DatabaseID    string `json:"database_id"`
	DestinationID string `json:"destination_id"`
}

type restoreBackupRequest struct {
	TargetInstanceID string `json:"target_instance_id"`
	TargetDatabase   string `json:"target_database"`
}

type backupResponse struct {
	ID            string     `json:"id"`
	DatabaseID    string     `json:"database_id"`
	DatabaseName  string     `json:"database_name,omitempty"`
	InstanceName  string     `json:"instance_name,omitempty"`
	JobID         *string    `json:"operation_id,omitempty"`
	DestinationID *string    `json:"destination_id,omitempty"`
	Type          string     `json:"type"`
	Engine        string     `json:"engine"`
	Status        string     `json:"status"`
	StorageURL    *string    `json:"storage_url,omitempty"`
	SizeBytes     *int64     `json:"size_bytes,omitempty"`
	Checksum      *string    `json:"checksum,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	Error         *string    `json:"error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	VerifyStatus  *string    `json:"verify_status,omitempty"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
	VerifyError   *string    `json:"verify_error,omitempty"`
}

func toBackupResponse(b *backupdom.Backup) backupResponse {
	var jobID, destID *string
	if b.JobID != nil {
		s := b.JobID.String()
		jobID = &s
	}
	if b.DestinationID != nil {
		s := b.DestinationID.String()
		destID = &s
	}
	return backupResponse{
		DatabaseName:  b.DatabaseName,
		InstanceName:  b.InstanceName,
		ID:            b.ID.String(),
		DatabaseID:    b.DatabaseID.String(),
		JobID:         jobID,
		DestinationID: destID,
		Type:          b.Type,
		Engine:        b.Engine,
		Status:        string(b.Status),
		StorageURL:    b.StorageURL,
		SizeBytes:     b.SizeBytes,
		Checksum:      b.Checksum,
		StartedAt:     b.StartedAt,
		CompletedAt:   b.CompletedAt,
		Error:         b.Error,
		CreatedAt:     b.CreatedAt,
		VerifyStatus:  b.VerifyStatus,
		VerifiedAt:    b.VerifiedAt,
		VerifyError:   b.VerifyError,
	}
}

// Trigger handles POST /v1/backups.
func (h *BackupHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	var req triggerBackupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	dbID, err := uuid.Parse(req.DatabaseID)
	if err != nil {
		writeError(w, apperr.Invalid("database_id", "must be a valid UUID"))
		return
	}
	if err := authorizeResource(r.Context(), h.resolver, "backup:write", authz.ResourceDatabase, dbID); err != nil {
		writeError(w, err)
		return
	}
	b, job, err := h.svc.Trigger(r.Context(), backupapp.TriggerInput{
		DatabaseID:    req.DatabaseID,
		DestinationID: req.DestinationID,
		CreatedBy:     callerID(r),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	resp := toBackupResponse(b)
	jid := job.ID.String()
	resp.JobID = &jid
	writeJSON(w, http.StatusCreated, resp)
}

// Restore handles POST /v1/backups/{id}/restore.
func (h *BackupHandler) Restore(w http.ResponseWriter, r *http.Request) {
	var req restoreBackupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	job, err := h.svc.Restore(r.Context(), backupapp.RestoreInput{
		BackupID:         r.PathValue("id"),
		TargetInstanceID: req.TargetInstanceID,
		TargetDatabase:   req.TargetDatabase,
		CreatedBy:        callerID(r),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"operation_id": job.ID.String()})
}

// List handles GET /v1/backups.
func (h *BackupHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.List(r.Context(), backupapp.ListParams{
		DatabaseID: q.Get("database_id"),
		Search:     q.Get("search"),
		Limit:      atoiDefault(q.Get("limit"), 0),
		Offset:     atoiDefault(q.Get("offset"), 0),
		Scope:      readScope(r.Context(), "backup:read"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]backupResponse, 0, len(res.Items))
	for _, b := range res.Items {
		items = append(items, toBackupResponse(b))
	}
	writeJSON(w, http.StatusOK, paginated(items, res.Total, res.Limit, res.Offset))
}

// Get handles GET /v1/backups/{id}.
func (h *BackupHandler) Get(w http.ResponseWriter, r *http.Request) {
	b, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toBackupResponse(b))
}

// Download handles GET /v1/backups/{id}/download: a short-lived presigned
// link to the gzipped SQL dump, fetched straight from the bucket.
func (h *BackupHandler) Download(w http.ResponseWriter, r *http.Request) {
	url, exp, err := h.svc.DownloadURL(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": url, "expires_at": exp})
}

// Delete handles DELETE /v1/backups/{id}.
func (h *BackupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// Verify handles POST /v1/backups/{id}/verify.
func (h *BackupHandler) Verify(w http.ResponseWriter, r *http.Request) {
	job, err := h.svc.Verify(r.Context(), r.PathValue("id"), callerID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"operation_id": job.ID.String()})
}

// maxSQLImportBytes caps an uploaded SQL file.
const maxSQLImportBytes = 2 << 30

// ImportSQL handles POST /v1/databases/{id}/import-sql?destination_id=&gzip=.
// The body is the SQL file (plain or gzip). It is staged in the destination
// bucket and applied by an operation running as the database's read-write
// role. Requires database:write and backup:write on the database.
func (h *BackupHandler) ImportSQL(w http.ResponseWriter, r *http.Request) {
	did, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, apperr.Invalid("id", "must be a valid UUID"))
		return
	}
	if err := authorizeResource(r.Context(), h.resolver, "backup:write", authz.ResourceDatabase, did); err != nil {
		writeError(w, err)
		return
	}
	if h.access != nil {
		if err := authorizeDataAccess(r.Context(), h.resolver, h.access, did); err != nil {
			writeError(w, err)
			return
		}
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(time.Hour))
	_ = rc.SetWriteDeadline(time.Now().Add(time.Hour))
	q := r.URL.Query()
	job, err := h.svc.ImportSQL(r.Context(), backupapp.ImportInput{
		DatabaseID:    did.String(),
		DestinationID: q.Get("destination_id"),
		Body:          http.MaxBytesReader(w, r.Body, maxSQLImportBytes),
		Gzipped:       q.Get("gzip") == "true",
		CreatedBy:     callerID(r),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"operation_id": job.ID.String()})
}
