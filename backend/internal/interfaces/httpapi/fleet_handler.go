package httpapi

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"

	fleetapp "github.com/Fleetdock/fleetdock/backend/internal/app/fleet"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// FleetHandler exposes fleet queries: one script run on many databases. A
// run is visible only to the user who started it; each database is
// authorized when the run is created and again before it runs there.
type FleetHandler struct {
	svc *fleetapp.Service
}

// NewFleetHandler builds the handler.
func NewFleetHandler(svc *fleetapp.Service) *FleetHandler { return &FleetHandler{svc: svc} }

type createFleetRunRequest struct {
	SQL            string   `json:"sql"`
	DatabaseIDs    []string `json:"database_ids"`
	Concurrency    int      `json:"concurrency"`
	StopOnError    *bool    `json:"stop_on_error"`
	Canary         *bool    `json:"canary"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	RowLimit       int      `json:"row_limit"`
}

func callerOrFail(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	p := principalFrom(r.Context())
	if p == nil {
		writeError(w, apperr.Unauthorized("authentication required"))
		return uuid.Nil, false
	}
	return p.UserID, true
}

// Create handles POST /v1/fleet-runs.
func (h *FleetHandler) Create(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	if p == nil {
		writeError(w, apperr.Unauthorized("authentication required"))
		return
	}
	var req createFleetRunRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	run, err := h.svc.Create(r.Context(), p, fleetapp.CreateInput{
		SQL: req.SQL, DatabaseIDs: req.DatabaseIDs, Concurrency: req.Concurrency, StopOnError: req.StopOnError,
		Canary: req.Canary, TimeoutSeconds: req.TimeoutSeconds, RowLimit: req.RowLimit,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// List handles GET /v1/fleet-runs.
func (h *FleetHandler) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerOrFail(w, r)
	if !ok {
		return
	}
	items, err := h.svc.List(r.Context(), uid)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// runAction resolves the caller and the {id} path value.
func runAction(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	uid, ok := callerOrFail(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		writeError(w, err)
		return uuid.Nil, uuid.Nil, false
	}
	return uid, id, true
}

// Get handles GET /v1/fleet-runs/{id}.
func (h *FleetHandler) Get(w http.ResponseWriter, r *http.Request) {
	uid, id, ok := runAction(w, r)
	if !ok {
		return
	}
	run, err := h.svc.Get(r.Context(), uid, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// Targets handles GET /v1/fleet-runs/{id}/targets.
func (h *FleetHandler) Targets(w http.ResponseWriter, r *http.Request) {
	uid, id, ok := runAction(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Targets(r.Context(), uid, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ResultsCSV handles GET /v1/fleet-runs/{id}/results.csv.
func (h *FleetHandler) ResultsCSV(w http.ResponseWriter, r *http.Request) {
	uid, id, ok := runAction(w, r)
	if !ok {
		return
	}
	if _, err := h.svc.Get(r.Context(), uid, id); err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="fleet-run-%s.csv"`, id.String()[:8]))
	if err := h.svc.WriteCSV(r.Context(), uid, id, w); err != nil {
		// Headers are sent; the client sees a truncated file.
		return
	}
}

func (h *FleetHandler) act(w http.ResponseWriter, r *http.Request,
	fn func(svc *fleetapp.Service, uid, id uuid.UUID) (any, error)) {
	uid, id, ok := runAction(w, r)
	if !ok {
		return
	}
	out, err := fn(h.svc, uid, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Resume handles POST /v1/fleet-runs/{id}/resume.
func (h *FleetHandler) Resume(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, func(s *fleetapp.Service, uid, id uuid.UUID) (any, error) { return s.Resume(r.Context(), uid, id) })
}

// Cancel handles POST /v1/fleet-runs/{id}/cancel.
func (h *FleetHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, func(s *fleetapp.Service, uid, id uuid.UUID) (any, error) { return s.Cancel(r.Context(), uid, id) })
}

// Retry handles POST /v1/fleet-runs/{id}/retry.
func (h *FleetHandler) Retry(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, func(s *fleetapp.Service, uid, id uuid.UUID) (any, error) { return s.Retry(r.Context(), uid, id) })
}
