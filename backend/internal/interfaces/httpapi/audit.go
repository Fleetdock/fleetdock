package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/Fleetdock/fleetdock/backend/internal/app/audit"
	auditdom "github.com/Fleetdock/fleetdock/backend/internal/domain/audit"
)

// clientIP is the caller's address. With trustProxy it is the last
// X-Forwarded-For entry: the one the trusted proxy in front of Fleetdock
// added for the peer it saw. Earlier entries come from the client (a proxy
// that appends, like nginx's $proxy_add_x_forwarded_for, keeps whatever the
// client sent), so taking the first would let a caller pick the address it
// is logged and rate-limited under. Without trustProxy it is the transport
// peer.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
			parts := strings.Split(xff[len(xff)-1], ",")
			if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
				return last
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// auditLog writes one audit event per request to an audited route. It sits
// between authentication and the mux, so it sees the caller, and it looks
// the route up on the mux before running it.
type auditLog struct {
	svc        *auditapp.Service
	mux        *http.ServeMux
	routes     map[string]auditRoute
	trustProxy bool
}

const maxCapturedResponse = 64 << 10

func (a *auditLog) Handler(next http.Handler) http.Handler {
	if a.svc == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := a.mux.Handler(r)
		spec, ok := a.routes[pattern]
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		req := a.request(r)
		ctx := auditapp.WithRequest(r.Context(), req)
		ctx, entry := auditapp.WithEntry(ctx)
		r = r.WithContext(ctx)

		params := pathParams(pattern, r.URL.Path)
		var resID *uuid.UUID
		var resName string
		if spec.idParam != "" {
			if id, err := uuid.Parse(params[spec.idParam]); err == nil {
				resID = &id
				// Before the handler: a deleted resource has no name after.
				resName = a.svc.ResourceName(ctx, spec.resourceType, id)
			}
		}

		rec := &auditWriter{ResponseWriter: w, status: http.StatusOK, capture: spec.fromResponse}
		next.ServeHTTP(rec, r)

		st := entry.State()
		if st.Recorded || st.Skip {
			return
		}
		outcome := ""
		switch {
		case st.Failed && rec.status == http.StatusUnauthorized:
			outcome = auditdom.OutcomeFailure
		case rec.status >= 200 && rec.status < 300:
			outcome = auditdom.OutcomeSuccess
		case rec.status == http.StatusForbidden:
			outcome = auditdom.OutcomeDenied
		default:
			return // validation errors, conflicts, outages: nothing happened
		}

		ev := &auditdom.Event{
			ActorUserID: req.UserID, ActorEmail: req.Email, ActorKind: req.Kind, TokenID: req.TokenID,
			IP: req.IP, UserAgent: req.UserAgent,
			Action: spec.action, Outcome: outcome, ResourceType: spec.resourceType,
			ResourceID: resID, ResourceName: resName,
		}
		if st.Actor != nil {
			ev.ActorUserID, ev.ActorEmail, ev.ActorKind = st.Actor.UserID, st.Actor.Email, auditdom.ActorUser
			if st.Actor.UserID == nil {
				ev.ActorKind = auditdom.ActorAnonymous
			}
		}
		if spec.resourceType == "user" && spec.idParam == "" && !spec.fromResponse {
			ev.ResourceID = ev.ActorUserID // sessions and profile act on the caller
			ev.ResourceName = ev.ActorEmail
		}
		if outcome == auditdom.OutcomeSuccess && spec.fromResponse && ev.ResourceID == nil {
			ev.ResourceID, ev.ResourceName = createdResource(rec.body.Bytes())
		}
		if st.ResourceID != nil {
			ev.ResourceID = st.ResourceID
		}
		if st.ResourceName != "" {
			ev.ResourceName = st.ResourceName
		}
		if ev.ResourceName == "" && ev.ResourceID != nil && outcome == auditdom.OutcomeSuccess {
			ev.ResourceName = a.svc.ResourceName(ctx, spec.resourceType, *ev.ResourceID)
		}
		meta := st.Meta
		for k, v := range params {
			if k != spec.idParam {
				meta[k] = v
			}
		}

		// The response is already sent: record even if the client went away.
		wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := a.svc.Append(wctx, ev, meta); err != nil {
			slog.Warn("audit event not recorded", "action", spec.action, "error", err)
		}
	})
}

func (a *auditLog) request(r *http.Request) auditapp.Request {
	req := auditapp.Request{IP: clientIP(r, a.trustProxy), UserAgent: r.UserAgent(), Kind: auditdom.ActorAnonymous}
	if p := principalFrom(r.Context()); p != nil {
		id := p.UserID
		req.UserID, req.Email, req.Kind = &id, p.Email, auditdom.ActorUser
		if p.TokenID != nil {
			req.TokenID, req.Kind = p.TokenID, auditdom.ActorAPIToken
		}
	}
	return req
}

// pathParams matches a mux pattern ("POST /v1/users/{id}") against a path
// and returns its wildcards. Patterns here have no multi-segment wildcards.
func pathParams(pattern, path string) map[string]string {
	out := map[string]string{}
	if _, p, ok := strings.Cut(pattern, " "); ok {
		pattern = p
	}
	ps := strings.Split(strings.Trim(pattern, "/"), "/")
	segs := strings.Split(strings.Trim(path, "/"), "/")
	if len(ps) != len(segs) {
		return out
	}
	for i, p := range ps {
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			out[strings.TrimSuffix(strings.TrimPrefix(p, "{"), "}")] = segs[i]
		}
	}
	return out
}

// createdResource reads the id and a display name of a created object from
// a JSON response. Only these fields are read: responses that also carry a
// secret (a new API token) never reach the log.
func createdResource(body []byte) (*uuid.UUID, string) {
	var v struct {
		ID          string `json:"id"`
		OperationID string `json:"operation_id"`
		Name        string `json:"name"`
		Email       string `json:"email"`
		Hostname    string `json:"hostname"`
	}
	if len(body) == 0 || json.Unmarshal(body, &v) != nil {
		return nil, ""
	}
	raw := v.ID
	if raw == "" {
		raw = v.OperationID
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, ""
	}
	name := v.Email // a user: the email names them better than the display name
	if name == "" {
		name = v.Name
	}
	if name == "" {
		name = v.Hostname
	}
	return &id, name
}

// auditWriter records the status and, for routes that create something, the
// start of the response body.
type auditWriter struct {
	http.ResponseWriter
	status  int
	capture bool
	body    bytes.Buffer
}

func (w *auditWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *auditWriter) Write(p []byte) (int, error) {
	if w.capture && w.body.Len() < maxCapturedResponse {
		n := min(len(p), maxCapturedResponse-w.body.Len())
		w.body.Write(p[:n])
	}
	return w.ResponseWriter.Write(p)
}

// Unwrap lets http.ResponseController reach Flush on streamed downloads.
func (w *auditWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
