package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	authapp "github.com/TajBrains/db-manager/backend/internal/app/auth"
	authzapp "github.com/TajBrains/db-manager/backend/internal/app/authz"
	authz "github.com/TajBrains/db-manager/backend/internal/domain/authz"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// Authenticator resolves the request principal from the Authorization header
// for all non-public routes.
type Authenticator struct {
	svc    *authapp.Service
	public map[string]bool
}

// NewAuthenticator builds the authentication middleware.
func NewAuthenticator(svc *authapp.Service) *Authenticator {
	return &Authenticator{
		svc: svc,
		public: map[string]bool{
			"/healthz":       true,
			"/v1/auth/login": true,
			"/openapi.yaml":  true,
			"/docs":          true,
		},
	}
}

// Middleware authenticates the request and injects the principal into context.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /agent/ and /install.sh are excluded: the agent protocol carries
		// its own bearer-token authentication.
		if a.public[r.URL.Path] || r.URL.Path == "/install.sh" || strings.HasPrefix(r.URL.Path, "/agent/") {
			next.ServeHTTP(w, r)
			return
		}
		p, err := a.svc.Principal(r.Context(), bearerToken(r))
		if err != nil {
			writeError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), p)))
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return ""
}

// requirePerm wraps a handler to require authentication and, if perm is
// non-empty, that permission at global scope. Use it for non-resource / admin
// routes (users, roles, tokens, audit, notifications, destinations, schedules).
func requirePerm(perm string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := principalFrom(r.Context())
		if p == nil {
			writeError(w, apperr.Unauthorized("authentication required"))
			return
		}
		if perm != "" && !p.Can(perm) {
			writeError(w, apperr.Forbidden("insufficient permissions"))
			return
		}
		h(w, r)
	}
}

// requireAnyPerm requires perm at any scope (global or scoped). Use it for list
// endpoints, which then filter their results to the caller's readable scope.
func requireAnyPerm(perm string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := principalFrom(r.Context())
		if p == nil {
			writeError(w, apperr.Unauthorized("authentication required"))
			return
		}
		if !p.CanAny(perm) {
			writeError(w, apperr.Forbidden("insufficient permissions"))
			return
		}
		h(w, r)
	}
}

// requireResourcePerm requires perm on the resource named by the {idParam} path
// value, resolving the resource's scope ancestry via the resolver.
func requireResourcePerm(rv *authzapp.Resolver, perm string, resType authz.ResourceType, idParam string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := principalFrom(r.Context())
		if p == nil {
			writeError(w, apperr.Unauthorized("authentication required"))
			return
		}
		id, err := uuid.Parse(r.PathValue(idParam))
		if err != nil {
			writeError(w, apperr.Invalid(idParam, "must be a valid UUID"))
			return
		}
		anc, err := rv.Ancestry(r.Context(), resType, id)
		if err != nil {
			writeError(w, err)
			return
		}
		if !p.CanOn(perm, anc) {
			writeError(w, apperr.Forbidden("insufficient permissions"))
			return
		}
		h(w, r)
	}
}

// readScope returns the caller's readable-scope restriction for perm, or nil
// when the caller may read everything (a global grant). A non-nil result (even
// empty) restricts the query to the listed server/database ids.
func readScope(ctx context.Context, perm string) *authz.ReadSet {
	p := principalFrom(ctx)
	if p == nil {
		return &authz.ReadSet{} // authenticated middleware runs first; defensive
	}
	rs := p.Readable(perm)
	if rs.All {
		return nil
	}
	return &rs
}

// authorizeResource is the handler-level equivalent of requireResourcePerm for
// body-referenced resources (e.g. creating a database on a target instance). It
// returns nil when the principal may act, otherwise a typed apperr.
func authorizeResource(ctx context.Context, rv *authzapp.Resolver, perm string, resType authz.ResourceType, resID uuid.UUID) error {
	p := principalFrom(ctx)
	if p == nil {
		return apperr.Unauthorized("authentication required")
	}
	anc, err := rv.Ancestry(ctx, resType, resID)
	if err != nil {
		return err
	}
	if !p.CanOn(perm, anc) {
		return apperr.Forbidden("insufficient permissions")
	}
	return nil
}
