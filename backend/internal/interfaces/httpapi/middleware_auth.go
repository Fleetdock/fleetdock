package httpapi

import (
	"net/http"
	"strings"

	authapp "github.com/TajBrains/db-manager/backend/internal/app/auth"
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
// non-empty, a specific permission.
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
