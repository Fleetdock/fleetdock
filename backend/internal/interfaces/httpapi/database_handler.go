package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	authzapp "github.com/Fleetdock/fleetdock/backend/internal/app/authz"
	databaseapp "github.com/Fleetdock/fleetdock/backend/internal/app/database"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// DatabaseHandler exposes database endpoints.
type DatabaseHandler struct {
	svc      *databaseapp.Service
	resolver *authzapp.Resolver
}

// NewDatabaseHandler builds the database handler.
func NewDatabaseHandler(svc *databaseapp.Service, resolver *authzapp.Resolver) *DatabaseHandler {
	return &DatabaseHandler{svc: svc, resolver: resolver}
}

type createDatabaseRequest struct {
	InstanceID string            `json:"instance_id"`
	Name       string            `json:"name"`
	Charset    string            `json:"charset"`
	Collation  string            `json:"collation"`
	Labels     map[string]string `json:"labels"`
	Tags       []string          `json:"tags"`
}

// instanceRefResponse is the owning-instance summary embedded in database
// responses so clients don't need a second, separately paginated request to
// render an instance name.
type instanceRefResponse struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Engine         string  `json:"engine"`
	Kind           string  `json:"kind"`
	ServerID       *string `json:"server_id,omitempty"`
	Provisioned    bool    `json:"provisioned"`
	HasCredentials bool    `json:"has_credentials"`
}

type databaseResponse struct {
	ID string `json:"id"`
	// InstanceID is retained for backwards compatibility; new clients should
	// read the embedded instance object.
	InstanceID        string               `json:"instance_id"`
	Instance          *instanceRefResponse `json:"instance,omitempty"`
	Name              string               `json:"name"`
	Charset           string               `json:"charset"`
	Collation         string               `json:"collation"`
	Status            string               `json:"status"`
	System            bool                 `json:"system"`
	SizeBytes         int64                `json:"size_bytes"`
	ActiveConnections int                  `json:"active_connections"`
	LockedAt          *time.Time           `json:"locked_at,omitempty"`
	MissingSince      *time.Time           `json:"missing_since,omitempty"`
	LockedBy          *string              `json:"locked_by,omitempty"`
	Labels            map[string]string    `json:"labels"`
	Tags              []string             `json:"tags"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
	Version           int                  `json:"version"`
}

func toDatabaseResponse(d *databasedom.Database) databaseResponse {
	var lockedBy *string
	if d.LockedBy != nil {
		s := d.LockedBy.String()
		lockedBy = &s
	}
	var inst *instanceRefResponse
	if d.Instance != nil {
		var serverID *string
		if d.Instance.ServerID != nil {
			s := d.Instance.ServerID.String()
			serverID = &s
		}
		inst = &instanceRefResponse{
			ID:             d.Instance.ID.String(),
			Name:           d.Instance.Name,
			Engine:         d.Instance.Engine,
			Kind:           d.Instance.Kind,
			ServerID:       serverID,
			Provisioned:    d.Instance.Provisioned,
			HasCredentials: d.Instance.HasCredentials,
		}
	}
	return databaseResponse{
		ID:                d.ID.String(),
		InstanceID:        d.InstanceID.String(),
		Instance:          inst,
		Name:              d.Name,
		Charset:           d.Charset,
		Collation:         d.Collation,
		Status:            string(d.Status),
		System:            d.System,
		SizeBytes:         d.SizeBytes,
		ActiveConnections: d.ActiveConnections,
		LockedAt:          d.LockedAt,
		MissingSince:      d.MissingSince,
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
	instID, err := uuid.Parse(req.InstanceID)
	if err != nil {
		writeError(w, apperr.Invalid("instance_id", "must be a valid UUID"))
		return
	}
	if err := authorizeResource(r.Context(), h.resolver, "database:write", authz.ResourceInstance, instID); err != nil {
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
		Scope:      readScope(r.Context(), "database:read"),
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

// Delete handles DELETE /v1/databases/{id}. Query param drop=true enqueues
// a physical DROP DATABASE on the instance (requires admin credentials).
func (h *DatabaseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	dropPhysical := r.URL.Query().Get("drop") == "true"
	var createdBy *uuid.UUID
	if p := principalFrom(r.Context()); p != nil {
		createdBy = &p.UserID
	}
	if err := h.svc.Delete(r.Context(), r.PathValue("id"), dropPhysical, createdBy); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
