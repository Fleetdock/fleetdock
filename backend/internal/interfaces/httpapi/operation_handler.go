package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	operationapp "github.com/TajBrains/fleetdock/backend/internal/app/operation"
	jobdom "github.com/TajBrains/fleetdock/backend/internal/domain/job"
)

// OperationHandler exposes read access to the operations log.
type OperationHandler struct {
	svc *operationapp.Service
}

// NewOperationHandler builds the operations handler.
func NewOperationHandler(svc *operationapp.Service) *OperationHandler {
	return &OperationHandler{svc: svc}
}

type operationResponse struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	ResourceType string          `json:"resource_type"`
	ResourceID   *string         `json:"resource_id,omitempty"`
	Status       string          `json:"status"`
	ServerID     *string         `json:"server_id,omitempty"`
	Params       json.RawMessage `json:"params,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
	Error        *string         `json:"error,omitempty"`
	Progress     int             `json:"progress"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

func toOperationResponse(j *jobdom.Job) operationResponse {
	var resourceID, serverID *string
	if j.ResourceID != nil {
		s := j.ResourceID.String()
		resourceID = &s
	}
	if j.ServerID != nil {
		s := j.ServerID.String()
		serverID = &s
	}
	return operationResponse{
		ID:           j.ID.String(),
		Type:         string(j.Type),
		ResourceType: j.ResourceType,
		ResourceID:   resourceID,
		Status:       string(j.Status),
		ServerID:     serverID,
		Params:       j.Params,
		Result:       j.Result,
		Error:        j.Error,
		Progress:     j.Progress,
		StartedAt:    j.StartedAt,
		CompletedAt:  j.CompletedAt,
		CreatedAt:    j.CreatedAt,
	}
}

// List handles GET /v1/operations.
func (h *OperationHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// Callers without global operation:read see only operations they created.
	var createdBy *uuid.UUID
	if p := principalFrom(r.Context()); p != nil && !p.Can("operation:read") {
		createdBy = &p.UserID
	}
	res, err := h.svc.List(r.Context(), operationapp.ListParams{
		Status:     q.Get("status"),
		Type:       q.Get("type"),
		ResourceID: q.Get("resource_id"),
		Limit:      atoiDefault(q.Get("limit"), 0),
		Offset:     atoiDefault(q.Get("offset"), 0),
		CreatedBy:  createdBy,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]operationResponse, 0, len(res.Items))
	for _, j := range res.Items {
		items = append(items, toOperationResponse(j))
	}
	writeJSON(w, http.StatusOK, paginated(items, res.Total, res.Limit, res.Offset))
}

// Get handles GET /v1/operations/{id}.
func (h *OperationHandler) Get(w http.ResponseWriter, r *http.Request) {
	j, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOperationResponse(j))
}

type operationLogResponse struct {
	Seq       int       `json:"seq"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// Logs handles GET /v1/operations/{id}/logs. Supports ?after_seq= for
// incremental tailing and ?limit= (default 500) to cap the batch size.
func (h *OperationHandler) Logs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	logs, err := h.svc.Logs(r.Context(), r.PathValue("id"),
		atoiDefault(q.Get("after_seq"), 0), atoiDefault(q.Get("limit"), 0))
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]operationLogResponse, 0, len(logs))
	for _, l := range logs {
		items = append(items, operationLogResponse{Seq: l.Seq, Level: l.Level, Message: l.Message, CreatedAt: l.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
