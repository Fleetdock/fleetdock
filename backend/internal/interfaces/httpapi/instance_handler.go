package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	instanceapp "github.com/mariadb-cp/db-manager/backend/internal/app/instance"
	instancedom "github.com/mariadb-cp/db-manager/backend/internal/domain/instance"
)

// InstanceHandler exposes instance endpoints.
type InstanceHandler struct {
	svc *instanceapp.Service
}

// NewInstanceHandler builds the instance handler.
func NewInstanceHandler(svc *instanceapp.Service) *InstanceHandler { return &InstanceHandler{svc: svc} }

type registerInstanceRequest struct {
	Kind          string `json:"kind"` // managed (default) | external
	ServerID      string `json:"server_id"`
	Host          string `json:"host"`
	Name          string `json:"name"`
	Engine        string `json:"engine"`
	EngineVersion string `json:"engine_version"`
	// Back-compat alias for engine_version.
	MariaDBVersion string            `json:"mariadb_version"`
	Port           int               `json:"port"`
	Username       string            `json:"username"`
	Password       string            `json:"password"` // write-only
	Labels         map[string]string `json:"labels"`
	Tags           []string          `json:"tags"`
}

type instanceResponse struct {
	ID             string            `json:"id"`
	ServerID       *string           `json:"server_id,omitempty"`
	Name           string            `json:"name"`
	Engine         string            `json:"engine"`
	Kind           string            `json:"kind"`
	Host           *string           `json:"host,omitempty"`
	Username       *string           `json:"username,omitempty"`
	HasCredentials bool              `json:"has_credentials"`
	ContainerID    *string           `json:"container_id,omitempty"`
	EngineVersion  string            `json:"engine_version"`
	MariaDBVersion string            `json:"mariadb_version"` // back-compat
	Port           int               `json:"port"`
	Status         string            `json:"status"`
	Labels         map[string]string `json:"labels"`
	Tags           []string          `json:"tags"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Version        int               `json:"version"`
}

func toInstanceResponse(in *instancedom.Instance) instanceResponse {
	var serverID *string
	if in.ServerID != nil {
		s := in.ServerID.String()
		serverID = &s
	}
	return instanceResponse{
		ID:             in.ID.String(),
		ServerID:       serverID,
		Name:           in.Name,
		Engine:         string(in.Engine),
		Kind:           string(in.Kind),
		Host:           in.Host,
		Username:       in.Username,
		HasCredentials: in.HasCredentials(),
		ContainerID:    in.ContainerID,
		EngineVersion:  in.EngineVersion,
		MariaDBVersion: in.EngineVersion,
		Port:           in.Port,
		Status:         string(in.Status),
		Labels:         in.Labels,
		Tags:           in.Tags,
		CreatedAt:      in.CreatedAt,
		UpdatedAt:      in.UpdatedAt,
		Version:        in.Version,
	}
}

// Register handles POST /v1/instances.
func (h *InstanceHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerInstanceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	engineVersion := req.EngineVersion
	if engineVersion == "" {
		engineVersion = req.MariaDBVersion
	}
	in, err := h.svc.Register(r.Context(), instanceapp.RegisterInput{
		Kind:          req.Kind,
		ServerID:      req.ServerID,
		Host:          req.Host,
		Name:          req.Name,
		Engine:        req.Engine,
		EngineVersion: engineVersion,
		Port:          req.Port,
		Username:      req.Username,
		Password:      req.Password,
		Labels:        req.Labels,
		Tags:          req.Tags,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toInstanceResponse(in))
}

// Get handles GET /v1/instances/{id}.
func (h *InstanceHandler) Get(w http.ResponseWriter, r *http.Request) {
	in, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toInstanceResponse(in))
}

// Delete handles DELETE /v1/instances/{id}.
func (h *InstanceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// List handles GET /v1/instances.
func (h *InstanceHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.List(r.Context(), instanceapp.ListParams{
		ServerID: q.Get("server_id"),
		Kind:     q.Get("kind"),
		Limit:    atoiDefault(q.Get("limit"), 0),
		Offset:   atoiDefault(q.Get("offset"), 0),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]instanceResponse, 0, len(res.Items))
	for _, in := range res.Items {
		items = append(items, toInstanceResponse(in))
	}
	writeJSON(w, http.StatusOK, paginated(items, res.Total, res.Limit, res.Offset))
}

// TestConnection handles POST /v1/instances/{id}/test-connection.
func (h *InstanceHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.TestConnection(r.Context(), r.PathValue("id"), callerID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":         res.Mode,
		"ok":           res.OK,
		"version":      res.Version,
		"error":        res.Error,
		"operation_id": res.OperationID,
	})
}

// ImportDatabases handles POST /v1/instances/{id}/import-databases.
func (h *InstanceHandler) ImportDatabases(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ImportDatabases(r.Context(), r.PathValue("id"), callerID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":         res.Mode,
		"imported":     res.Imported,
		"operation_id": res.OperationID,
	})
}

// callerID extracts the authenticated user's id, if any.
func callerID(r *http.Request) *uuid.UUID {
	if p := principalFrom(r.Context()); p != nil {
		id := p.UserID
		return &id
	}
	return nil
}
