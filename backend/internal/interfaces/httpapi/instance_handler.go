package httpapi

import (
	"net/http"
	"time"

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
	ServerID       string            `json:"server_id"`
	Name           string            `json:"name"`
	MariaDBVersion string            `json:"mariadb_version"`
	Port           int               `json:"port"`
	Labels         map[string]string `json:"labels"`
	Tags           []string          `json:"tags"`
}

type instanceResponse struct {
	ID             string            `json:"id"`
	ServerID       string            `json:"server_id"`
	Name           string            `json:"name"`
	ContainerID    *string           `json:"container_id,omitempty"`
	MariaDBVersion string            `json:"mariadb_version"`
	Port           int               `json:"port"`
	Status         string            `json:"status"`
	Labels         map[string]string `json:"labels"`
	Tags           []string          `json:"tags"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Version        int               `json:"version"`
}

func toInstanceResponse(in *instancedom.Instance) instanceResponse {
	return instanceResponse{
		ID:             in.ID.String(),
		ServerID:       in.ServerID.String(),
		Name:           in.Name,
		ContainerID:    in.ContainerID,
		MariaDBVersion: in.MariaDBVersion,
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
	in, err := h.svc.Register(r.Context(), instanceapp.RegisterInput{
		ServerID:       req.ServerID,
		Name:           req.Name,
		MariaDBVersion: req.MariaDBVersion,
		Port:           req.Port,
		Labels:         req.Labels,
		Tags:           req.Tags,
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

// List handles GET /v1/instances.
func (h *InstanceHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.List(r.Context(), instanceapp.ListParams{
		ServerID: q.Get("server_id"),
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
