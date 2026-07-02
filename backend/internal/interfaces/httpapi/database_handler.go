package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	databaseapp "github.com/mariadb-cp/db-manager/backend/internal/app/database"
	databasedom "github.com/mariadb-cp/db-manager/backend/internal/domain/database"
)

// DatabaseHandler exposes database endpoints.
type DatabaseHandler struct {
	svc *databaseapp.Service
}

// NewDatabaseHandler builds the database handler.
func NewDatabaseHandler(svc *databaseapp.Service) *DatabaseHandler { return &DatabaseHandler{svc: svc} }

type createDatabaseRequest struct {
	InstanceID string            `json:"instance_id"`
	Name       string            `json:"name"`
	Charset    string            `json:"charset"`
	Collation  string            `json:"collation"`
	Labels     map[string]string `json:"labels"`
	Tags       []string          `json:"tags"`
}

type databaseResponse struct {
	ID                string            `json:"id"`
	InstanceID        string            `json:"instance_id"`
	Name              string            `json:"name"`
	Charset           string            `json:"charset"`
	Collation         string            `json:"collation"`
	Status            string            `json:"status"`
	SizeBytes         int64             `json:"size_bytes"`
	ActiveConnections int               `json:"active_connections"`
	LockedAt          *time.Time        `json:"locked_at,omitempty"`
	LockedBy          *string           `json:"locked_by,omitempty"`
	Labels            map[string]string `json:"labels"`
	Tags              []string          `json:"tags"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	Version           int               `json:"version"`
}

func toDatabaseResponse(d *databasedom.Database) databaseResponse {
	var lockedBy *string
	if d.LockedBy != nil {
		s := d.LockedBy.String()
		lockedBy = &s
	}
	return databaseResponse{
		ID:                d.ID.String(),
		InstanceID:        d.InstanceID.String(),
		Name:              d.Name,
		Charset:           d.Charset,
		Collation:         d.Collation,
		Status:            string(d.Status),
		SizeBytes:         d.SizeBytes,
		ActiveConnections: d.ActiveConnections,
		LockedAt:          d.LockedAt,
		LockedBy:          lockedBy,
		Labels:            d.Labels,
		Tags:              d.Tags,
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
		Version:           d.Version,
	}
}

// Create handles POST /v1/databases.
func (h *DatabaseHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createDatabaseRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	var createdBy *uuid.UUID
	if p := principalFrom(r.Context()); p != nil {
		createdBy = &p.UserID
	}
	d, err := h.svc.Create(r.Context(), databaseapp.CreateInput{
		InstanceID: req.InstanceID,
		Name:       req.Name,
		Charset:    req.Charset,
		Collation:  req.Collation,
		Labels:     req.Labels,
		Tags:       req.Tags,
	}, createdBy)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toDatabaseResponse(d))
}

// Get handles GET /v1/databases/{id}.
func (h *DatabaseHandler) Get(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDatabaseResponse(d))
}

// List handles GET /v1/databases.
func (h *DatabaseHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.List(r.Context(), databaseapp.ListParams{
		InstanceID: q.Get("instance_id"),
		Status:     q.Get("status"),
		Search:     q.Get("search"),
		Limit:      atoiDefault(q.Get("limit"), 0),
		Offset:     atoiDefault(q.Get("offset"), 0),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]databaseResponse, 0, len(res.Items))
	for _, d := range res.Items {
		items = append(items, toDatabaseResponse(d))
	}
	writeJSON(w, http.StatusOK, paginated(items, res.Total, res.Limit, res.Offset))
}

// Lock handles POST /v1/databases/{id}/lock.
func (h *DatabaseHandler) Lock(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	d, err := h.svc.Lock(r.Context(), r.PathValue("id"), p.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDatabaseResponse(d))
}

// Unlock handles POST /v1/databases/{id}/unlock.
func (h *DatabaseHandler) Unlock(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.Unlock(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDatabaseResponse(d))
}

// Delete handles DELETE /v1/databases/{id}.
func (h *DatabaseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
