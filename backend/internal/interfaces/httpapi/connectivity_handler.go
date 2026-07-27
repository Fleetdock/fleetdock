package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	endpointapp "github.com/Fleetdock/fleetdock/backend/internal/app/endpoint"
)

type ConnectivityHandler struct {
	svc *endpointapp.Service
}

// NewConnectivityHandler builds the handler.
func NewConnectivityHandler(svc *endpointapp.Service) *ConnectivityHandler {
	return &ConnectivityHandler{svc: svc}
}

type enablePublicAccessRequest struct {
	AllowedCIDRs   []string `json:"allowed_cidrs"`
	TLSMode        string   `json:"tls_mode"`
	MaxConnections *int     `json:"max_connections"`
}

type publicAccessResponse struct {
	Endpoint    *endpointapp.EndpointView `json:"endpoint"`
	OperationID string                    `json:"operation_id"`
}

type disablePublicAccessResponse struct {
	OperationID string `json:"operation_id"`
}

type updateAllowedCIDRsRequest struct {
	AllowedCIDRs []string `json:"allowed_cidrs"`
}

// GetConnectivity handles GET /v1/databases/{id}/connectivity.
func (h *ConnectivityHandler) GetConnectivity(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.GetConnectivity(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// EnablePublicAccess handles POST /v1/databases/{id}/public-access.
func (h *ConnectivityHandler) EnablePublicAccess(w http.ResponseWriter, r *http.Request) {
	var req enablePublicAccessRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	var actor *uuid.UUID
	if p := principalFrom(r.Context()); p != nil {
		actor = &p.UserID
	}
	out, err := h.svc.EnablePublicAccess(r.Context(), r.PathValue("id"), endpointapp.EnableInput{
		AllowedCIDRs:   req.AllowedCIDRs,
		TLSMode:        req.TLSMode,
		MaxConnections: req.MaxConnections,
	}, actor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, publicAccessResponse{Endpoint: out.Endpoint, OperationID: out.OperationID})
}

// DisablePublicAccess handles DELETE /v1/databases/{id}/public-access.
func (h *ConnectivityHandler) DisablePublicAccess(w http.ResponseWriter, r *http.Request) {
	var actor *uuid.UUID
	if p := principalFrom(r.Context()); p != nil {
		actor = &p.UserID
	}
	opID, err := h.svc.DisablePublicAccess(r.Context(), r.PathValue("id"), actor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, disablePublicAccessResponse{OperationID: opID})
}

// UpdateAllowedCIDRs handles PATCH /v1/databases/{id}/public-access.
func (h *ConnectivityHandler) UpdateAllowedCIDRs(w http.ResponseWriter, r *http.Request) {
	var req updateAllowedCIDRsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	var actor *uuid.UUID
	if p := principalFrom(r.Context()); p != nil {
		actor = &p.UserID
	}
	opID, err := h.svc.UpdateAllowedCIDRs(r.Context(), r.PathValue("id"), req.AllowedCIDRs, actor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, disablePublicAccessResponse{OperationID: opID})
}
