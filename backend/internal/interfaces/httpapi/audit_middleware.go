package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	auditapp "github.com/mariadb-cp/db-manager/backend/internal/app/audit"
	auditdom "github.com/mariadb-cp/db-manager/backend/internal/domain/audit"
)

// auditRecorder records successful mutating requests to the audit log. It runs
// inside the authenticated chain so the principal is available, and records
// asynchronously so it never adds latency or blocks the response.
func auditRecorder(svc *auditapp.Service, next http.Handler) http.Handler {
	if svc == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutating(r.Method) || !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		// Only record successful mutations; skip auth (login) noise.
		if sw.status < 200 || sw.status >= 300 || r.URL.Path == "/v1/auth/login" {
			return
		}

		action, resourceType, resourceID := describeRequest(r.Method, r.URL.Path)
		in := auditapp.RecordInput{
			ActorType:    auditdom.ActorSystem,
			Action:       action,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			Metadata: map[string]any{
				"method": r.Method,
				"path":   r.URL.Path,
				"status": sw.status,
				"ip":     clientIP(r),
			},
		}
		if p := principalFrom(r.Context()); p != nil {
			id := p.UserID
			in.ActorType = auditdom.ActorUser
			in.ActorID = &id
			in.Metadata["actor_email"] = p.Email
		}
		// Detach from the request context so recording survives the response.
		go func() {
			_ = svc.Record(context.Background(), in)
		}()
	})
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

// describeRequest turns a method+path into an action string with :id
// placeholders, plus the resource type and first resource UUID in the path.
// e.g. DELETE /v1/databases/<uuid> -> ("DELETE /v1/databases/:id", "databases", <uuid>).
func describeRequest(method, path string) (action, resourceType string, resourceID *uuid.UUID) {
	segments := strings.Split(strings.Trim(strings.TrimPrefix(path, "/v1"), "/"), "/")
	pattern := make([]string, 0, len(segments))
	for _, seg := range segments {
		if id, err := uuid.Parse(seg); err == nil {
			pattern = append(pattern, ":id")
			if resourceID == nil {
				rid := id
				resourceID = &rid
			}
			continue
		}
		pattern = append(pattern, seg)
	}
	if len(segments) > 0 {
		resourceType = segments[0]
	}
	action = method + " /v1/" + strings.Join(pattern, "/")
	return action, resourceType, resourceID
}

// statusWriter captures the response status code.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}
