package httpapi

import (
	"net/http"
	"time"

	destinationapp "github.com/TajBrains/fleetdock/backend/internal/app/destination"
	backupdestdom "github.com/TajBrains/fleetdock/backend/internal/domain/backupdest"
)

// DestinationHandler exposes backup-destination endpoints.
type DestinationHandler struct {
	svc *destinationapp.Service
}

// NewDestinationHandler builds the destination handler.
func NewDestinationHandler(svc *destinationapp.Service) *DestinationHandler {
	return &DestinationHandler{svc: svc}
}

type createDestinationRequest struct {
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	Endpoint        string `json:"endpoint"`
	Prefix          string `json:"prefix"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"` // write-only
}

type updateDestinationRequest struct {
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	Endpoint        string `json:"endpoint"`
	Prefix          string `json:"prefix"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"` // optional; omit or leave empty to keep existing
}

type destinationResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Provider    string    `json:"provider"`
	Bucket      string    `json:"bucket"`
	Region      string    `json:"region,omitempty"`
	Endpoint    string    `json:"endpoint,omitempty"`
	Prefix      string    `json:"prefix,omitempty"`
	AccessKeyID string    `json:"access_key_id"`
	CreatedAt   time.Time `json:"created_at"`
}

func toDestinationResponse(d *backupdestdom.Destination) destinationResponse {
	return destinationResponse{
		ID:          d.ID.String(),
		Name:        d.Name,
		Provider:    string(d.Provider),
		Bucket:      d.Bucket,
		Region:      d.Region,
		Endpoint:    d.Endpoint,
		Prefix:      d.Prefix,
		AccessKeyID: d.AccessKeyID,
		CreatedAt:   d.CreatedAt,
	}
}

// Create handles POST /v1/backup-destinations.
func (h *DestinationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createDestinationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	d, err := h.svc.Create(r.Context(), destinationapp.CreateInput{
		Name:            req.Name,
		Provider:        req.Provider,
		Bucket:          req.Bucket,
		Region:          req.Region,
		Endpoint:        req.Endpoint,
		Prefix:          req.Prefix,
		AccessKeyID:     req.AccessKeyID,
		SecretAccessKey: req.SecretAccessKey,
		CreatedBy:       callerID(r),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toDestinationResponse(d))
}

// List handles GET /v1/backup-destinations.
func (h *DestinationHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]destinationResponse, 0, len(items))
	for _, d := range items {
		out = append(out, toDestinationResponse(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// Update handles PATCH /v1/backup-destinations/{id}.
func (h *DestinationHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req updateDestinationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	d, err := h.svc.Update(r.Context(), destinationapp.UpdateInput{
		ID:              r.PathValue("id"),
		Name:            req.Name,
		Provider:        req.Provider,
		Bucket:          req.Bucket,
		Region:          req.Region,
		Endpoint:        req.Endpoint,
		Prefix:          req.Prefix,
		AccessKeyID:     req.AccessKeyID,
		SecretAccessKey: req.SecretAccessKey,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDestinationResponse(d))
}

// Delete handles DELETE /v1/backup-destinations/{id}.
func (h *DestinationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// Test handles POST /v1/backup-destinations/{id}/test.
func (h *DestinationHandler) Test(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Test(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
