package httpapi

import (
	"net/http"
	"time"

	scheduleapp "github.com/TajBrains/fleetdock/backend/internal/app/schedule"
	scheduledom "github.com/TajBrains/fleetdock/backend/internal/domain/schedule"
)

// ScheduleHandler exposes backup-schedule endpoints.
type ScheduleHandler struct {
	svc *scheduleapp.Service
}

// NewScheduleHandler builds the schedule handler.
func NewScheduleHandler(svc *scheduleapp.Service) *ScheduleHandler { return &ScheduleHandler{svc: svc} }

type createScheduleRequest struct {
	DatabaseID    string `json:"database_id"`
	DestinationID string `json:"destination_id"`
	Cron          string `json:"cron"`
	RetentionDays int    `json:"retention_days"`
	Enabled       *bool  `json:"enabled"`
}

type updateScheduleRequest struct {
	DestinationID string `json:"destination_id"`
	Cron          string `json:"cron"`
	RetentionDays int    `json:"retention_days"`
	Enabled       *bool  `json:"enabled"`
}

type scheduleResponse struct {
	ID            string     `json:"id"`
	DatabaseID    string     `json:"database_id"`
	DestinationID string     `json:"destination_id"`
	Cron          string     `json:"cron"`
	Engine        string     `json:"engine"`
	RetentionDays int        `json:"retention_days"`
	Enabled       bool       `json:"enabled"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

func toScheduleResponse(s *scheduledom.Schedule) scheduleResponse {
	return scheduleResponse{
		ID:            s.ID.String(),
		DatabaseID:    s.DatabaseID.String(),
		DestinationID: s.DestinationID.String(),
		Cron:          s.Cron,
		Engine:        s.Engine,
		RetentionDays: s.RetentionDays,
		Enabled:       s.Enabled,
		LastRunAt:     s.LastRunAt,
		NextRunAt:     s.NextRunAt,
		CreatedAt:     s.CreatedAt,
	}
}

// Create handles POST /v1/backup-schedules.
func (h *ScheduleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createScheduleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	s, err := h.svc.Create(r.Context(), scheduleapp.CreateInput{
		DatabaseID:    req.DatabaseID,
		DestinationID: req.DestinationID,
		Cron:          req.Cron,
		RetentionDays: req.RetentionDays,
		Enabled:       enabled,
		CreatedBy:     callerID(r),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toScheduleResponse(s))
}

// List handles GET /v1/backup-schedules.
func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), r.URL.Query().Get("database_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]scheduleResponse, 0, len(items))
	for _, s := range items {
		out = append(out, toScheduleResponse(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// Update handles PATCH /v1/backup-schedules/{id}.
func (h *ScheduleHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req updateScheduleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	s, err := h.svc.Update(r.Context(), scheduleapp.UpdateInput{
		ID:            r.PathValue("id"),
		DestinationID: req.DestinationID,
		Cron:          req.Cron,
		RetentionDays: req.RetentionDays,
		Enabled:       enabled,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toScheduleResponse(s))
}

// Delete handles DELETE /v1/backup-schedules/{id}.
func (h *ScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
