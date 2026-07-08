package httpapi

import (
	"net/http"

	moveapp "github.com/TajBrains/db-manager/backend/internal/app/move"
)

// MoveHandler exposes the move-database action. A move has no resource of its
// own: it is tracked through the backup and restore operations it creates.
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

// Start handles POST /v1/moves. It kicks off the move and returns the backup
// operation that begins it; the restore operation follows automatically.
func (h *MoveHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req startMoveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	job, err := h.svc.Start(r.Context(), moveapp.StartInput{
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
	writeJSON(w, http.StatusAccepted, map[string]string{
		"operation_id": job.ID.String(),
		"status":       string(job.Status),
	})
}
