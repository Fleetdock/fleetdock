package httpapi

import (
	"context"
	"net/http"
	"time"
)

// RouterDeps are the handlers and middleware the router wires together.
type RouterDeps struct {
	Auth         *AuthHandler
	Servers      *ServerHandler
	Instances    *InstanceHandler
	Databases    *DatabaseHandler
	Tokens       *TokenHandler
	Users        *UserHandler
	Operations   *OperationHandler
	Backups      *BackupHandler
	Destinations *DestinationHandler
	DBAdmin      *DBAdminHandler
	Agents       *AgentHandler
	RegTokens    *RegTokenHandler
	Install      *InstallHandler
	Authn        *Authenticator
	CORSOrigin   string
	// Ready reports whether dependencies (the metadata database) are healthy.
	Ready func(ctx context.Context) error
}

// NewRouter builds the HTTP handler tree with authentication, RBAC, CORS,
// logging and panic recovery applied. It uses the Go 1.22 method+pattern
// ServeMux (no external router dependency).
func NewRouter(d RouterDeps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if d.Ready != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			if err := d.Ready(ctx); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	// Public: agent install script + binaries.
	mux.HandleFunc("GET /install.sh", d.Install.Script)
	mux.HandleFunc("GET /agent/v1/binary/{os}/{arch}", d.Install.Binary)

	// Agent protocol (agent bearer token, not user JWT).
	mux.HandleFunc("POST /agent/v1/register", d.Agents.Register)
	mux.HandleFunc("POST /agent/v1/heartbeat", d.Agents.Auth(d.Agents.Heartbeat))
	mux.HandleFunc("POST /agent/v1/jobs/claim", d.Agents.Auth(d.Agents.Claim))
	mux.HandleFunc("POST /agent/v1/jobs/{id}/status", d.Agents.Auth(d.Agents.UpdateJob))

	// Auth (login is rate limited per client IP)
	limiter := newLoginLimiter(10, time.Minute)
	mux.HandleFunc("POST /v1/auth/login", limiter.Middleware(d.Auth.Login))
	mux.HandleFunc("GET /v1/auth/me", requirePerm("", d.Auth.Me))

	// Self-service profile
	mux.HandleFunc("GET /v1/profile", requirePerm("", d.Users.Profile))
	mux.HandleFunc("PATCH /v1/profile", requirePerm("", d.Users.UpdateProfile))
	mux.HandleFunc("POST /v1/profile/password", requirePerm("", d.Users.ChangePassword))

	// User administration + role catalog
	mux.HandleFunc("GET /v1/users", requirePerm("user:read", d.Users.List))
	mux.HandleFunc("POST /v1/users", requirePerm("user:write", d.Users.Create))
	mux.HandleFunc("PATCH /v1/users/{id}", requirePerm("user:write", d.Users.Update))
	mux.HandleFunc("POST /v1/users/{id}/password", requirePerm("user:write", d.Users.ResetPassword))
	mux.HandleFunc("DELETE /v1/users/{id}", requirePerm("user:write", d.Users.Delete))
	mux.HandleFunc("GET /v1/roles", requirePerm("user:read", d.Users.ListRoles))
	mux.HandleFunc("POST /v1/roles", requirePerm("user:write", d.Users.CreateRole))
	mux.HandleFunc("PATCH /v1/roles/{id}", requirePerm("user:write", d.Users.UpdateRole))
	mux.HandleFunc("DELETE /v1/roles/{id}", requirePerm("user:write", d.Users.DeleteRole))
	mux.HandleFunc("GET /v1/permissions", requirePerm("user:read", d.Users.ListPermissions))

	// Servers
	mux.HandleFunc("POST /v1/servers", requirePerm("server:write", d.Servers.Register))
	mux.HandleFunc("GET /v1/servers", requirePerm("server:read", d.Servers.List))
	mux.HandleFunc("GET /v1/servers/{id}", requirePerm("server:read", d.Servers.Get))

	// Agent registration tokens (server connect flow)
	mux.HandleFunc("POST /v1/agent-tokens", requirePerm("server:write", d.RegTokens.Create))
	mux.HandleFunc("GET /v1/agent-tokens", requirePerm("server:read", d.RegTokens.List))
	mux.HandleFunc("DELETE /v1/agent-tokens/{id}", requirePerm("server:write", d.RegTokens.Revoke))

	// Instances (managed + external)
	mux.HandleFunc("POST /v1/instances", requirePerm("instance:write", d.Instances.Register))
	mux.HandleFunc("GET /v1/instances", requirePerm("instance:read", d.Instances.List))
	mux.HandleFunc("GET /v1/instances/{id}", requirePerm("instance:read", d.Instances.Get))
	mux.HandleFunc("DELETE /v1/instances/{id}", requirePerm("instance:write", d.Instances.Delete))
	mux.HandleFunc("POST /v1/instances/{id}/test-connection", requirePerm("instance:read", d.Instances.TestConnection))
	mux.HandleFunc("POST /v1/instances/{id}/import-databases", requirePerm("instance:write", d.Instances.ImportDatabases))

	// Databases
	mux.HandleFunc("POST /v1/databases", requirePerm("database:write", d.Databases.Create))
	mux.HandleFunc("GET /v1/databases", requirePerm("database:read", d.Databases.List))
	mux.HandleFunc("GET /v1/databases/{id}", requirePerm("database:read", d.Databases.Get))
	mux.HandleFunc("POST /v1/databases/{id}/lock", requirePerm("database:write", d.Databases.Lock))
	mux.HandleFunc("POST /v1/databases/{id}/unlock", requirePerm("database:write", d.Databases.Unlock))
	mux.HandleFunc("DELETE /v1/databases/{id}", requirePerm("database:write", d.Databases.Delete))

	// Live DB administration: accounts + grants (instance scope)
	mux.HandleFunc("GET /v1/instances/{id}/db-users", requirePerm("instance:read", d.DBAdmin.ListDBUsers))
	mux.HandleFunc("POST /v1/instances/{id}/db-users", requirePerm("instance:write", d.DBAdmin.CreateDBUser))
	mux.HandleFunc("POST /v1/instances/{id}/db-users/drop", requirePerm("instance:write", d.DBAdmin.DropDBUser))
	mux.HandleFunc("GET /v1/instances/{id}/db-users/grants", requirePerm("instance:read", d.DBAdmin.UserGrants))
	mux.HandleFunc("POST /v1/instances/{id}/grants", requirePerm("instance:write", d.DBAdmin.Grant))
	mux.HandleFunc("POST /v1/instances/{id}/grants/revoke", requirePerm("instance:write", d.DBAdmin.Revoke))

	// Live DB administration: database scope (grants, tables, data)
	mux.HandleFunc("GET /v1/databases/{id}/grants", requirePerm("database:read", d.DBAdmin.SchemaGrants))
	mux.HandleFunc("POST /v1/databases/{id}/grants", requirePerm("database:write", d.DBAdmin.GrantOnDatabase))
	mux.HandleFunc("POST /v1/databases/{id}/grants/revoke", requirePerm("database:write", d.DBAdmin.RevokeOnDatabase))
	mux.HandleFunc("GET /v1/databases/{id}/db-users", requirePerm("database:read", d.DBAdmin.ListDBUsersForDatabase))
	mux.HandleFunc("GET /v1/databases/{id}/tables", requirePerm("database:read", d.DBAdmin.ListTables))
	mux.HandleFunc("GET /v1/databases/{id}/tables/{table}/rows", requirePerm("database:read", d.DBAdmin.TableRows))
	mux.HandleFunc("GET /v1/db-privileges", requirePerm("instance:read", d.DBAdmin.ListPrivileges))

	// Operations (jobs)
	mux.HandleFunc("GET /v1/operations", requirePerm("operation:read", d.Operations.List))
	mux.HandleFunc("GET /v1/operations/{id}", requirePerm("operation:read", d.Operations.Get))

	// Backups + restore
	mux.HandleFunc("POST /v1/backups", requirePerm("backup:write", d.Backups.Trigger))
	mux.HandleFunc("GET /v1/backups", requirePerm("backup:read", d.Backups.List))
	mux.HandleFunc("GET /v1/backups/{id}", requirePerm("backup:read", d.Backups.Get))
	mux.HandleFunc("POST /v1/backups/{id}/restore", requirePerm("backup:write", d.Backups.Restore))

	// Backup destinations
	mux.HandleFunc("POST /v1/backup-destinations", requirePerm("destination:write", d.Destinations.Create))
	mux.HandleFunc("GET /v1/backup-destinations", requirePerm("destination:read", d.Destinations.List))
	mux.HandleFunc("PATCH /v1/backup-destinations/{id}", requirePerm("destination:write", d.Destinations.Update))
	mux.HandleFunc("DELETE /v1/backup-destinations/{id}", requirePerm("destination:write", d.Destinations.Delete))
	mux.HandleFunc("POST /v1/backup-destinations/{id}/test", requirePerm("destination:write", d.Destinations.Test))

	// API tokens (scoped to the caller)
	mux.HandleFunc("POST /v1/tokens", requirePerm("token:write", d.Tokens.Create))
	mux.HandleFunc("GET /v1/tokens", requirePerm("token:read", d.Tokens.List))
	mux.HandleFunc("DELETE /v1/tokens/{id}", requirePerm("token:write", d.Tokens.Revoke))

	// Middleware order (outermost first):
	// logging -> recover -> security headers -> CORS -> auth -> mux.
	return requestLogger(recoverer(securityHeaders(cors(d.CORSOrigin, d.Authn.Middleware(mux)))))
}
