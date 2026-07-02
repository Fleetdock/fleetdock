package httpapi

import (
	"net/http"
	"time"

	backupapp "github.com/mariadb-cp/db-manager/backend/internal/app/backup"
	backupdom "github.com/mariadb-cp/db-manager/backend/internal/domain/backup"
)

// BackupHandler exposes backup + restore endpoints.
type BackupHandler struct {
	svc *backupapp.Service
}

// NewBackupHandler builds the backup handler.
func NewBackupHandler(svc *backupapp.Service) *BackupHandler { return &BackupHandler{svc: svc} }

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
	}
}

// Trigger handles POST /v1/backups.
func (h *BackupHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	var req triggerBackupRequest
	if err := decodeJSON(r, &req); err != nil {
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
		Limit:      atoiDefault(q.Get("limit"), 0),
		Offset:     atoiDefault(q.Get("offset"), 0),
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
