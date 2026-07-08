package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	serverapp "github.com/TajBrains/db-manager/backend/internal/app/server"
	serverdom "github.com/TajBrains/db-manager/backend/internal/domain/server"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// ServersService is the use-case surface the HTTP layer depends on
// (consumer-defined interface; *serverapp.Service satisfies it).
type ServersService interface {
	Register(ctx context.Context, in serverapp.RegisterInput) (*serverdom.Server, error)
	Get(ctx context.Context, id string) (*serverdom.Server, error)
	List(ctx context.Context, p serverapp.ListParams) (serverapp.ListResult, error)
}

// ServerHandler exposes server endpoints.
type ServerHandler struct {
	svc ServersService
}

// NewServerHandler builds a handler over the given service.
func NewServerHandler(svc ServersService) *ServerHandler { return &ServerHandler{svc: svc} }

// ---- DTOs ----

type registerServerRequest struct {
	Name     string            `json:"name"`
	Hostname string            `json:"hostname"`
	Address  *string           `json:"address"`
	OS       *string           `json:"os"`
	Labels   map[string]string `json:"labels"`
	Tags     []string          `json:"tags"`
}

type serverResponse struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Hostname        string            `json:"hostname"`
	Address         *string           `json:"address,omitempty"`
	Status          string            `json:"status"`
	AgentVersion    *string           `json:"agent_version,omitempty"`
	MariaDBVersion  *string           `json:"mariadb_version,omitempty"`
	OS              *string           `json:"os,omitempty"`
	Labels          map[string]string `json:"labels"`
	Tags            []string          `json:"tags"`
	LastHeartbeatAt *time.Time        `json:"last_heartbeat_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	Version         int               `json:"version"`
}

type pagination struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type listServersResponse struct {
	Items      []serverResponse `json:"items"`
	Pagination pagination       `json:"pagination"`
}

func toServerResponse(s *serverdom.Server) serverResponse {
	return serverResponse{
		ID:              s.ID.String(),
		Name:            s.Name,
		Hostname:        s.Hostname,
		Address:         s.Address,
		Status:          string(s.Status),
		AgentVersion:    s.AgentVersion,
		MariaDBVersion:  s.MariaDBVersion,
		OS:              s.OS,
		Labels:          s.Labels,
		Tags:            s.Tags,
		LastHeartbeatAt: s.LastHeartbeatAt,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
		Version:         s.Version,
	}
}

// ---- Handlers ----

// Register handles POST /v1/servers.
func (h *ServerHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerServerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	srv, err := h.svc.Register(r.Context(), serverapp.RegisterInput{
		Name:     req.Name,
		Hostname: req.Hostname,
		Address:  req.Address,
		OS:       req.OS,
		Labels:   req.Labels,
		Tags:     req.Tags,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toServerResponse(srv))
}

// Get handles GET /v1/servers/{id}.
func (h *ServerHandler) Get(w http.ResponseWriter, r *http.Request) {
	srv, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toServerResponse(srv))
}

// List handles GET /v1/servers.
func (h *ServerHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.List(r.Context(), serverapp.ListParams{
		Status: q.Get("status"),
		Search: q.Get("search"),
		Tag:    q.Get("tag"),
		Limit:  atoiDefault(q.Get("limit"), 0),
		Offset: atoiDefault(q.Get("offset"), 0),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]serverResponse, 0, len(res.Items))
	for _, s := range res.Items {
		items = append(items, toServerResponse(s))
	}
	writeJSON(w, http.StatusOK, listServersResponse{
		Items:      items,
		Pagination: pagination{Total: res.Total, Limit: res.Limit, Offset: res.Offset},
	})
}

// ---- helpers ----

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20)) // 1 MiB cap
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apperr.Invalid("body", "request body must be valid JSON")
	}
	return nil
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
