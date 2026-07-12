package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	authzapp "github.com/Fleetdock/fleetdock/backend/internal/app/authz"
	instanceapp "github.com/Fleetdock/fleetdock/backend/internal/app/instance"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// InstanceHandler exposes instance endpoints.
type InstanceHandler struct {
	svc      *instanceapp.Service
	resolver *authzapp.Resolver
}

// NewInstanceHandler builds the instance handler.
func NewInstanceHandler(svc *instanceapp.Service, resolver *authzapp.Resolver) *InstanceHandler {
	return &InstanceHandler{svc: svc, resolver: resolver}
}

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
	Provisioned    bool              `json:"provisioned"`
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
		Provisioned:    in.Provisioned(),
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
	// Registering an instance under a managed server requires instance:write on
	// that server; external instances (no server) require it globally.
	if req.ServerID != "" {
		sid, err := uuid.Parse(req.ServerID)
		if err != nil {
			writeError(w, apperr.Invalid("server_id", "must be a valid UUID"))
			return
		}
		if err := authorizeResource(r.Context(), h.resolver, "instance:write", authz.ResourceServer, sid); err != nil {
			writeError(w, err)
			return
		}
	} else if p := principalFrom(r.Context()); p == nil || !p.Can("instance:write") {
		writeError(w, apperr.Forbidden("insufficient permissions"))
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

type provisionInstanceRequest struct {
	ServerID      string `json:"server_id"`
	Name          string `json:"name"`
	Engine        string `json:"engine"`
	EngineVersion string `json:"engine_version"`
	Port          int    `json:"port"`
}

// Provision handles POST /v1/instances/provision.
func (h *InstanceHandler) Provision(w http.ResponseWriter, r *http.Request) {
	var req provisionInstanceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	sid, err := uuid.Parse(req.ServerID)
	if err != nil {
		writeError(w, apperr.Invalid("server_id", "must be a valid UUID"))
		return
	}
	if err := authorizeResource(r.Context(), h.resolver, "instance:write", authz.ResourceServer, sid); err != nil {
		writeError(w, err)
		return
	}
	in, job, err := h.svc.Provision(r.Context(), instanceapp.ProvisionInput{
		ServerID:      req.ServerID,
		Name:          req.Name,
		Engine:        req.Engine,
		EngineVersion: req.EngineVersion,
		Port:          req.Port,
		CreatedBy:     callerID(r),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	resp := toInstanceResponse(in)
	writeJSON(w, http.StatusCreated, map[string]any{"instance": resp, "operation_id": job.ID.String()})
}

// Lifecycle handles POST /v1/instances/{id}/{action} for start|stop|restart.
func (h *InstanceHandler) Lifecycle(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		job, err := h.svc.Lifecycle(r.Context(), r.PathValue("id"), action, callerID(r))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"operation_id": job.ID.String()})
	}
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

// Delete handles DELETE /v1/instances/{id}?remove_volume=true.
func (h *InstanceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	removeVolume := r.URL.Query().Get("remove_volume") == "true"
	if err := h.svc.Delete(r.Context(), r.PathValue("id"), removeVolume, callerID(r)); err != nil {
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
		Scope:    readScope(r.Context(), "instance:read"),
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
