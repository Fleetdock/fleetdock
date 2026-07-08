package httpapi

import (
	"net/http"
	"time"

	moveapp "github.com/mariadb-cp/db-manager/backend/internal/app/move"
	movedom "github.com/mariadb-cp/db-manager/backend/internal/domain/move"
)

// MoveHandler exposes the move-database endpoints.
type MoveHandler struct {
	svc *moveapp.Service
}

// NewMoveHandler builds the move handler.
func NewMoveHandler(svc *moveapp.Service) *MoveHandler { return &MoveHandler{svc: svc} }

type startMoveRequest struct {
	SourceDatabaseID string `json:"source_database_id"`
	TargetInstanceID string `json:"target_instance_id"`
	TargetDatabase   string `json:"target_database"`
	DestinationID    string `json:"destination_id"`
	DropSource       bool   `json:"drop_source"`
}

type moveResponse struct {
	ID               string    `json:"id"`
	SourceDatabaseID string    `json:"source_database_id"`
	TargetInstanceID string    `json:"target_instance_id"`
	TargetDatabase   string    `json:"target_database"`
	DestinationID    string    `json:"destination_id"`
	DropSource       bool      `json:"drop_source"`
	BackupID         *string   `json:"backup_id,omitempty"`
	RestoreJobID     *string   `json:"restore_job_id,omitempty"`
	Status           string    `json:"status"`
	TableCount       *int      `json:"table_count,omitempty"`
	Error            *string   `json:"error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func toMoveResponse(m *movedom.Move) moveResponse {
	var backupID, restoreJobID *string
	if m.BackupID != nil {
		s := m.BackupID.String()
		backupID = &s
	}
	if m.RestoreJobID != nil {
		s := m.RestoreJobID.String()
		restoreJobID = &s
	}
	return moveResponse{
		ID:               m.ID.String(),
		SourceDatabaseID: m.SourceDatabaseID.String(),
		TargetInstanceID: m.TargetInstanceID.String(),
		TargetDatabase:   m.TargetDatabase,
		DestinationID:    m.DestinationID.String(),
		DropSource:       m.DropSource,
		BackupID:         backupID,
		RestoreJobID:     restoreJobID,
		Status:           string(m.Status),
		TableCount:       m.TableCount,
		Error:            m.Error,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
}

// Start handles POST /v1/moves.
func (h *MoveHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req startMoveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	m, err := h.svc.Start(r.Context(), moveapp.StartInput{
		SourceDatabaseID: req.SourceDatabaseID,
		TargetInstanceID: req.TargetInstanceID,
		TargetDatabase:   req.TargetDatabase,
		DestinationID:    req.DestinationID,
		DropSource:       req.DropSource,
		CreatedBy:        callerID(r),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, toMoveResponse(m))
}

// List handles GET /v1/moves.
func (h *MoveHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]moveResponse, 0, len(items))
	for _, m := range items {
		out = append(out, toMoveResponse(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// Get handles GET /v1/moves/{id}.
func (h *MoveHandler) Get(w http.ResponseWriter, r *http.Request) {
	m, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toMoveResponse(m))
}
