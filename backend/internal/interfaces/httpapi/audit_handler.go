package httpapi

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/Fleetdock/fleetdock/backend/internal/app/audit"
	auditdom "github.com/Fleetdock/fleetdock/backend/internal/domain/audit"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// AuditHandler serves the audit log.
type AuditHandler struct {
	svc *auditapp.Service
}

// List handles GET /v1/audit-events: newest first, filtered, a page at a
// time (cursor), or the whole selection as CSV with format=csv.
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		writeError(w, apperr.NotFound("the audit log is not enabled"))
		return
	}
	f, err := auditFilter(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if r.URL.Query().Get("format") == "csv" {
		items, err := h.svc.Export(r.Context(), f)
		if err != nil {
			writeError(w, err)
			return
		}
		writeAuditCSV(w, items)
		return
	}
	page, err := h.svc.List(r.Context(), f, r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func auditFilter(r *http.Request) (auditdom.Filter, error) {
	q := r.URL.Query()
	var f auditdom.Filter
	uuidParam := func(name string) (*uuid.UUID, error) {
		v := q.Get(name)
		if v == "" {
			return nil, nil
		}
		id, err := uuid.Parse(v)
		if err != nil {
			return nil, apperr.Invalid(name, "must be a valid UUID")
		}
		return &id, nil
	}
	timeParam := func(name string) (time.Time, error) {
		v := q.Get(name)
		if v == "" {
			return time.Time{}, nil
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, apperr.Invalid(name, "must be an RFC 3339 time")
		}
		return t, nil
	}
	var err error
	if f.ActorUserID, err = uuidParam("actor"); err != nil {
		return f, err
	}
	if f.ResourceID, err = uuidParam("resource_id"); err != nil {
		return f, err
	}
	if f.Since, err = timeParam("since"); err != nil {
		return f, err
	}
	if f.Until, err = timeParam("until"); err != nil {
		return f, err
	}
	f.ActionPrefix = strings.TrimSpace(q.Get("action"))
	f.ResourceType = strings.TrimSpace(q.Get("resource_type"))
	switch o := q.Get("outcome"); o {
	case "", auditdom.OutcomeSuccess, auditdom.OutcomeDenied, auditdom.OutcomeFailure:
		f.Outcome = o
	default:
		return f, apperr.Invalid("outcome", "must be success, denied or failure")
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return f, apperr.Invalid("limit", "must be a positive integer")
		}
		f.Limit = n
	}
	return f, nil
}

// csvSafe stops a spreadsheet from running a cell as a formula.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func writeAuditCSV(w http.ResponseWriter, items []auditdom.Event) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="fleetdock-audit-`+time.Now().UTC().Format("20060102-150405")+`.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"time", "actor_email", "actor_kind", "actor_user_id", "token_id", "action", "outcome",
		"resource_type", "resource_id", "resource_name", "ip", "user_agent", "metadata"})
	str := func(id *uuid.UUID) string {
		if id == nil {
			return ""
		}
		return id.String()
	}
	for _, e := range items {
		_ = cw.Write([]string{
			e.CreatedAt.UTC().Format(time.RFC3339Nano), csvSafe(e.ActorEmail), e.ActorKind, str(e.ActorUserID), str(e.TokenID),
			e.Action, e.Outcome, e.ResourceType, str(e.ResourceID), csvSafe(e.ResourceName), e.IP, csvSafe(e.UserAgent),
			csvSafe(string(e.Metadata)),
		})
	}
	cw.Flush()
}
