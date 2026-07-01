package httpapi

import "net/http"

// RouterDeps are the handlers and middleware the router wires together.
type RouterDeps struct {
	Auth       *AuthHandler
	Servers    *ServerHandler
	Instances  *InstanceHandler
	Databases  *DatabaseHandler
	Tokens     *TokenHandler
	Authn      *Authenticator
	CORSOrigin string
}

// NewRouter builds the HTTP handler tree with authentication, RBAC, CORS,
// logging and panic recovery applied. It uses the Go 1.22 method+pattern
// ServeMux (no external router dependency).
func NewRouter(d RouterDeps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Auth
	mux.HandleFunc("POST /v1/auth/login", d.Auth.Login)
	mux.HandleFunc("GET /v1/auth/me", requirePerm("", d.Auth.Me))

	// Servers
	mux.HandleFunc("POST /v1/servers", requirePerm("server:write", d.Servers.Register))
	mux.HandleFunc("GET /v1/servers", requirePerm("server:read", d.Servers.List))
	mux.HandleFunc("GET /v1/servers/{id}", requirePerm("server:read", d.Servers.Get))

	// Instances
	mux.HandleFunc("POST /v1/instances", requirePerm("instance:write", d.Instances.Register))
	mux.HandleFunc("GET /v1/instances", requirePerm("instance:read", d.Instances.List))
	mux.HandleFunc("GET /v1/instances/{id}", requirePerm("instance:read", d.Instances.Get))

	// Databases
	mux.HandleFunc("POST /v1/databases", requirePerm("database:write", d.Databases.Create))
	mux.HandleFunc("GET /v1/databases", requirePerm("database:read", d.Databases.List))
	mux.HandleFunc("GET /v1/databases/{id}", requirePerm("database:read", d.Databases.Get))
	mux.HandleFunc("POST /v1/databases/{id}/lock", requirePerm("database:write", d.Databases.Lock))
	mux.HandleFunc("POST /v1/databases/{id}/unlock", requirePerm("database:write", d.Databases.Unlock))
	mux.HandleFunc("DELETE /v1/databases/{id}", requirePerm("database:write", d.Databases.Delete))

	// API tokens (scoped to the caller)
	mux.HandleFunc("POST /v1/tokens", requirePerm("token:write", d.Tokens.Create))
	mux.HandleFunc("GET /v1/tokens", requirePerm("token:read", d.Tokens.List))
	mux.HandleFunc("DELETE /v1/tokens/{id}", requirePerm("token:write", d.Tokens.Revoke))

	// Middleware order (outermost first): logging -> recover -> CORS -> auth -> mux.
	return requestLogger(recoverer(cors(d.CORSOrigin, d.Authn.Middleware(mux))))
}
