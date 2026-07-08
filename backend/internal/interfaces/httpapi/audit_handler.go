package httpapi

import (
	"net/http"
	"time"

	auditapp "github.com/TajBrains/db-manager/backend/internal/app/audit"
	auditdom "github.com/TajBrains/db-manager/backend/internal/domain/audit"
)

// AuditHandler exposes the audit-log read endpoint.
type AuditHandler struct {
	svc *auditapp.Service
}

// NewAuditHandler builds the audit handler.
func NewAuditHandler(svc *auditapp.Service) *AuditHandler { return &AuditHandler{svc: svc} }

type auditResponse struct {
	ID           int64          `json:"id"`
	ActorType    string         `json:"actor_type"`
	ActorID      *string        `json:"actor_id,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   *string        `json:"resource_id,omitempty"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
}

func toAuditResponse(e *auditdom.Entry) auditResponse {
	var actorID, resourceID *string
	if e.ActorID != nil {
		s := e.ActorID.String()
		actorID = &s
	}
	if e.ResourceID != nil {
		s := e.ResourceID.String()
		resourceID = &s
	}
	meta := e.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	return auditResponse{
		ID:           e.ID,
		ActorType:    string(e.ActorType),
		ActorID:      actorID,
		Action:       e.Action,
		ResourceType: e.ResourceType,
		ResourceID:   resourceID,
		Metadata:     meta,
		CreatedAt:    e.CreatedAt,
	}
}

// List handles GET /v1/audit.
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.List(r.Context(), auditapp.ListParams{
		ActorID:      q.Get("actor_id"),
		ResourceType: q.Get("resource_type"),
		ResourceID:   q.Get("resource_id"),
		Limit:        atoiDefault(q.Get("limit"), 0),
		Offset:       atoiDefault(q.Get("offset"), 0),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]auditResponse, 0, len(res.Items))
	for _, e := range res.Items {
		items = append(items, toAuditResponse(e))
	}
	writeJSON(w, http.StatusOK, paginated(items, res.Total, res.Limit, res.Offset))
}
