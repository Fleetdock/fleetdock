package httpapi

import (
	"context"
	"net/http"
	"time"

	authzapp "github.com/Fleetdock/fleetdock/backend/internal/app/authz"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
)

// RouterDeps are the handlers and middleware the router wires together.
type RouterDeps struct {
	Auth          *AuthHandler
	Servers       *ServerHandler
	Instances     *InstanceHandler
	Databases     *DatabaseHandler
	Tokens        *TokenHandler
	Users         *UserHandler
	Operations    *OperationHandler
	Backups       *BackupHandler
	Schedules     *ScheduleHandler
	Moves         *MoveHandler
	Destinations  *DestinationHandler
	DBAdmin       *DBAdminHandler
	Connectivity  *ConnectivityHandler
	DBCredentials *DBCredentialHandler
	Agents        *AgentHandler
	RegTokens     *RegTokenHandler
	Install       *InstallHandler
	Notifications *NotificationHandler
	Overview      *OverviewHandler
	Docs          *DocsHandler
	Authn         *Authenticator
	// Resolver resolves resource scope ancestry for per-resource authorization.
	Resolver   *authzapp.Resolver
	CORSOrigin string
	// TrustProxyHeaders lets the login rate limiter read the client IP from
	// X-Forwarded-For (set only when behind a trusted reverse proxy).
	TrustProxyHeaders bool
	// Ready reports whether dependencies (the metadata database) are healthy.
	Ready func(ctx context.Context) error
	// UI, when non-nil, serves every path the API does not own: the bundled
	// dashboard, reverse-proxied to a co-resident Node process. Nil keeps the
	// API-only behaviour, where unknown paths 404.
	UI http.Handler
}

// NewRouter builds the HTTP handler tree with authentication, RBAC, CORS,
// logging and panic recovery applied. It uses the Go 1.22 method+pattern
// ServeMux (no external router dependency).
func NewRouter(d RouterDeps) http.Handler {
	mux := http.NewServeMux()
	rv := d.Resolver

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

	// Public: API documentation (spec + Redoc page).
	mux.HandleFunc("GET /openapi.yaml", d.Docs.Spec)
	mux.HandleFunc("GET /docs", d.Docs.Page)

	// Public: agent install script + binaries.
	mux.HandleFunc("GET /install.sh", d.Install.Script)
	mux.HandleFunc("GET /agent/v1/binary/{os}/{arch}", d.Install.Binary)

	// Agent protocol (agent bearer token, not user JWT).
	mux.HandleFunc("POST /agent/v1/register", d.Agents.Register)
	mux.HandleFunc("POST /agent/v1/heartbeat", d.Agents.Auth(d.Agents.Heartbeat))
	mux.HandleFunc("POST /agent/v1/jobs/claim", d.Agents.Auth(d.Agents.Claim))
	mux.HandleFunc("POST /agent/v1/jobs/{id}/status", d.Agents.Auth(d.Agents.UpdateJob))
	mux.HandleFunc("POST /agent/v1/jobs/{id}/logs", d.Agents.Auth(d.Agents.AppendLogs))

	// Auth (login is rate limited per client IP)
	limiter := newLoginLimiter(10, time.Minute, d.TrustProxyHeaders)
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
	mux.HandleFunc("GET /v1/users/{id}/role-grants", requirePerm("user:read", d.Users.ListGrants))
	mux.HandleFunc("POST /v1/users/{id}/role-grants", requirePerm("user:write", d.Users.AddGrant))
	mux.HandleFunc("DELETE /v1/users/{id}/role-grants/{grantId}", requirePerm("user:write", d.Users.RemoveGrant))
	mux.HandleFunc("GET /v1/roles", requirePerm("user:read", d.Users.ListRoles))
	mux.HandleFunc("POST /v1/roles", requirePerm("user:write", d.Users.CreateRole))
	mux.HandleFunc("PATCH /v1/roles/{id}", requirePerm("user:write", d.Users.UpdateRole))
	mux.HandleFunc("DELETE /v1/roles/{id}", requirePerm("user:write", d.Users.DeleteRole))
	mux.HandleFunc("GET /v1/permissions", requirePerm("user:read", d.Users.ListPermissions))

	// Overview dashboard (any authenticated user)
	mux.HandleFunc("GET /v1/overview", requirePerm("", d.Overview.Overview))

	// Servers
	mux.HandleFunc("POST /v1/servers", requirePerm("server:write", d.Servers.Register))
	mux.HandleFunc("GET /v1/servers", requireAnyPerm("server:read", d.Servers.List))
	mux.HandleFunc("GET /v1/servers/{id}", requireResourcePerm(rv, "server:read", authz.ResourceServer, "id", d.Servers.Get))
	mux.HandleFunc("PATCH /v1/servers/{id}", requireResourcePerm(rv, "server:write", authz.ResourceServer, "id", d.Servers.Update))
	mux.HandleFunc("DELETE /v1/servers/{id}", requireResourcePerm(rv, "server:write", authz.ResourceServer, "id", d.Servers.Delete))
	mux.HandleFunc("GET /v1/servers/{id}/metrics", requireResourcePerm(rv, "server:read", authz.ResourceServer, "id", d.Overview.ServerMetrics))

	// Agent registration tokens (server connect flow)
	mux.HandleFunc("POST /v1/agent-tokens", requirePerm("server:write", d.RegTokens.Create))
	mux.HandleFunc("GET /v1/agent-tokens", requirePerm("server:read", d.RegTokens.List))
	mux.HandleFunc("DELETE /v1/agent-tokens/{id}", requirePerm("server:write", d.RegTokens.Revoke))

	// Instances (managed + external). Register/Provision authorize the target
	// server inside the handler (external instances need it globally).
	mux.HandleFunc("POST /v1/instances", requireAnyPerm("instance:write", d.Instances.Register))
	mux.HandleFunc("POST /v1/instances/provision", requireAnyPerm("instance:write", d.Instances.Provision))
	mux.HandleFunc("GET /v1/instances", requireAnyPerm("instance:read", d.Instances.List))
	mux.HandleFunc("GET /v1/instances/{id}", requireResourcePerm(rv, "instance:read", authz.ResourceInstance, "id", d.Instances.Get))
	mux.HandleFunc("DELETE /v1/instances/{id}", requireResourcePerm(rv, "instance:write", authz.ResourceInstance, "id", d.Instances.Delete))
	mux.HandleFunc("POST /v1/instances/{id}/start", requireResourcePerm(rv, "instance:write", authz.ResourceInstance, "id", d.Instances.Lifecycle("start")))
	mux.HandleFunc("POST /v1/instances/{id}/stop", requireResourcePerm(rv, "instance:write", authz.ResourceInstance, "id", d.Instances.Lifecycle("stop")))
	mux.HandleFunc("POST /v1/instances/{id}/restart", requireResourcePerm(rv, "instance:write", authz.ResourceInstance, "id", d.Instances.Lifecycle("restart")))
	mux.HandleFunc("POST /v1/instances/{id}/test-connection", requireResourcePerm(rv, "instance:read", authz.ResourceInstance, "id", d.Instances.TestConnection))
	mux.HandleFunc("POST /v1/instances/{id}/import-databases", requireResourcePerm(rv, "instance:write", authz.ResourceInstance, "id", d.Instances.ImportDatabases))

	// Databases. Create authorizes the target instance inside the handler.
	mux.HandleFunc("POST /v1/databases", requireAnyPerm("database:write", d.Databases.Create))
	mux.HandleFunc("GET /v1/databases", requireAnyPerm("database:read", d.Databases.List))
	mux.HandleFunc("GET /v1/databases/{id}", requireResourcePerm(rv, "database:read", authz.ResourceDatabase, "id", d.Databases.Get))
	mux.HandleFunc("POST /v1/databases/{id}/lock", requireResourcePerm(rv, "database:write", authz.ResourceDatabase, "id", d.Databases.Lock))
	mux.HandleFunc("POST /v1/databases/{id}/unlock", requireResourcePerm(rv, "database:write", authz.ResourceDatabase, "id", d.Databases.Unlock))
	mux.HandleFunc("DELETE /v1/databases/{id}", requireResourcePerm(rv, "database:write", authz.ResourceDatabase, "id", d.Databases.Delete))

	// Database connectivity + application credentials.
	mux.HandleFunc("GET /v1/databases/{id}/connectivity", requireResourcePerm(rv, "database:read", authz.ResourceDatabase, "id", d.Connectivity.GetConnectivity))
	mux.HandleFunc("POST /v1/databases/{id}/public-access", requireResourcePerm(rv, "database:write", authz.ResourceDatabase, "id", d.Connectivity.EnablePublicAccess))
	mux.HandleFunc("PATCH /v1/databases/{id}/public-access", requireResourcePerm(rv, "database:write", authz.ResourceDatabase, "id", d.Connectivity.UpdateAllowedCIDRs))
	mux.HandleFunc("DELETE /v1/databases/{id}/public-access", requireResourcePerm(rv, "database:write", authz.ResourceDatabase, "id", d.Connectivity.DisablePublicAccess))
	mux.HandleFunc("GET /v1/databases/{id}/credentials", requireResourcePerm(rv, "database:read", authz.ResourceDatabase, "id", d.DBCredentials.List))
	mux.HandleFunc("POST /v1/databases/{id}/credentials", requireResourcePerm(rv, "database:write", authz.ResourceDatabase, "id", d.DBCredentials.Create))
	mux.HandleFunc("POST /v1/databases/{id}/credentials/{credentialId}/rotate", requireResourcePerm(rv, "database:write", authz.ResourceDatabase, "id", d.DBCredentials.Rotate))
	mux.HandleFunc("DELETE /v1/databases/{id}/credentials/{credentialId}", requireResourcePerm(rv, "database:write", authz.ResourceDatabase, "id", d.DBCredentials.Revoke))

	// Live DB administration: accounts + grants (instance scope)
	mux.HandleFunc("GET /v1/instances/{id}/db-users", requireResourcePerm(rv, "instance:read", authz.ResourceInstance, "id", d.DBAdmin.ListDBUsers))
	mux.HandleFunc("POST /v1/instances/{id}/db-users", requireResourcePerm(rv, "instance:write", authz.ResourceInstance, "id", d.DBAdmin.CreateDBUser))
	mux.HandleFunc("POST /v1/instances/{id}/db-users/drop", requireResourcePerm(rv, "instance:write", authz.ResourceInstance, "id", d.DBAdmin.DropDBUser))
	mux.HandleFunc("GET /v1/instances/{id}/db-users/grants", requireResourcePerm(rv, "instance:read", authz.ResourceInstance, "id", d.DBAdmin.UserGrants))
	mux.HandleFunc("POST /v1/instances/{id}/grants", requireResourcePerm(rv, "instance:write", authz.ResourceInstance, "id", d.DBAdmin.Grant))
	mux.HandleFunc("POST /v1/instances/{id}/grants/revoke", requireResourcePerm(rv, "instance:write", authz.ResourceInstance, "id", d.DBAdmin.Revoke))

	// Live DB administration: database scope (grants, tables, data)
	mux.HandleFunc("GET /v1/databases/{id}/grants", requireResourcePerm(rv, "database:read", authz.ResourceDatabase, "id", d.DBAdmin.SchemaGrants))
	mux.HandleFunc("POST /v1/databases/{id}/grants", requireResourcePerm(rv, "database:write", authz.ResourceDatabase, "id", d.DBAdmin.GrantOnDatabase))
	mux.HandleFunc("POST /v1/databases/{id}/grants/revoke", requireResourcePerm(rv, "database:write", authz.ResourceDatabase, "id", d.DBAdmin.RevokeOnDatabase))
	mux.HandleFunc("GET /v1/databases/{id}/db-users", requireResourcePerm(rv, "database:read", authz.ResourceDatabase, "id", d.DBAdmin.ListDBUsersForDatabase))
	mux.HandleFunc("GET /v1/databases/{id}/tables", requireResourcePerm(rv, "database:read", authz.ResourceDatabase, "id", d.DBAdmin.ListTables))
	mux.HandleFunc("GET /v1/databases/{id}/tables/{table}/rows", requireResourcePerm(rv, "database:read", authz.ResourceDatabase, "id", d.DBAdmin.TableRows))
	mux.HandleFunc("GET /v1/databases/{id}/tables/{table}/schema", requireResourcePerm(rv, "database:read", authz.ResourceDatabase, "id", d.DBAdmin.TableSchema))
	mux.HandleFunc("GET /v1/databases/{id}/tables/{table}/export", requireResourcePerm(rv, "database:read", authz.ResourceDatabase, "id", d.DBAdmin.ExportTable))
	// SQL console: any writes are gated inside the handler by database:write.
	mux.HandleFunc("POST /v1/databases/{id}/query", requireResourcePerm(rv, "database:read", authz.ResourceDatabase, "id", d.DBAdmin.Query))
	mux.HandleFunc("POST /v1/databases/{id}/export", requireResourcePerm(rv, "database:read", authz.ResourceDatabase, "id", d.DBAdmin.ExportQuery))
	mux.HandleFunc("GET /v1/db-privileges", requireAnyPerm("instance:read", d.DBAdmin.ListPrivileges))

	// Operations (jobs)
	mux.HandleFunc("GET /v1/operations", requireAnyPerm("operation:read", d.Operations.List))
	mux.HandleFunc("GET /v1/operations/{id}", requireResourcePerm(rv, "operation:read", authz.ResourceOperation, "id", d.Operations.Get))
	mux.HandleFunc("GET /v1/operations/{id}/logs", requireResourcePerm(rv, "operation:read", authz.ResourceOperation, "id", d.Operations.Logs))

	// Backups + restore
	mux.HandleFunc("POST /v1/backups", requireAnyPerm("backup:write", d.Backups.Trigger))
	mux.HandleFunc("GET /v1/backups", requireAnyPerm("backup:read", d.Backups.List))
	mux.HandleFunc("GET /v1/backups/{id}", requireResourcePerm(rv, "backup:read", authz.ResourceBackup, "id", d.Backups.Get))
	mux.HandleFunc("POST /v1/backups/{id}/restore", requireResourcePerm(rv, "backup:write", authz.ResourceBackup, "id", d.Backups.Restore))

	// Move database (backup → restore → verify → optional drop of source).
	// Authorizes source database + target instance inside the handler.
	mux.HandleFunc("POST /v1/moves", requireAnyPerm("backup:write", d.Moves.Start))

	// Backup destinations
	mux.HandleFunc("POST /v1/backup-destinations", requirePerm("destination:write", d.Destinations.Create))
	mux.HandleFunc("GET /v1/backup-destinations", requirePerm("destination:read", d.Destinations.List))
	mux.HandleFunc("PATCH /v1/backup-destinations/{id}", requirePerm("destination:write", d.Destinations.Update))
	mux.HandleFunc("DELETE /v1/backup-destinations/{id}", requirePerm("destination:write", d.Destinations.Delete))
	mux.HandleFunc("POST /v1/backup-destinations/{id}/test", requirePerm("destination:write", d.Destinations.Test))

	// Backup schedules
	mux.HandleFunc("POST /v1/backup-schedules", requirePerm("schedule:write", d.Schedules.Create))
	mux.HandleFunc("GET /v1/backup-schedules", requirePerm("schedule:read", d.Schedules.List))
	mux.HandleFunc("PATCH /v1/backup-schedules/{id}", requirePerm("schedule:write", d.Schedules.Update))
	mux.HandleFunc("DELETE /v1/backup-schedules/{id}", requirePerm("schedule:write", d.Schedules.Delete))

	// Notification channels
	mux.HandleFunc("POST /v1/notification-channels", requirePerm("notification:write", d.Notifications.CreateChannel))
	mux.HandleFunc("GET /v1/notification-channels", requirePerm("notification:read", d.Notifications.ListChannels))
	mux.HandleFunc("PATCH /v1/notification-channels/{id}", requirePerm("notification:write", d.Notifications.UpdateChannel))
	mux.HandleFunc("DELETE /v1/notification-channels/{id}", requirePerm("notification:write", d.Notifications.DeleteChannel))
	mux.HandleFunc("POST /v1/notification-channels/{id}/test", requirePerm("notification:write", d.Notifications.TestChannel))

	// Alert rules
	mux.HandleFunc("POST /v1/alert-rules", requirePerm("notification:write", d.Notifications.CreateRule))
	mux.HandleFunc("GET /v1/alert-rules", requirePerm("notification:read", d.Notifications.ListRules))
	mux.HandleFunc("PATCH /v1/alert-rules/{id}", requirePerm("notification:write", d.Notifications.UpdateRule))
	mux.HandleFunc("DELETE /v1/alert-rules/{id}", requirePerm("notification:write", d.Notifications.DeleteRule))

	// API tokens (scoped to the caller)
	mux.HandleFunc("POST /v1/tokens", requirePerm("token:write", d.Tokens.Create))
	mux.HandleFunc("GET /v1/tokens", requirePerm("token:read", d.Tokens.List))
	mux.HandleFunc("DELETE /v1/tokens/{id}", requirePerm("token:write", d.Tokens.Revoke))

	// Middleware order (outermost first):
	// logging -> recover -> [UI split] -> security headers -> CORS -> auth -> mux.
	//
	// CORS and the API's Cache-Control: no-store stay strictly inside the API
	// branch. The dashboard is served from this same origin so it needs no
	// preflight handling, and Next's own immutable caching for /_next/static
	// must survive — see uiproxy.Handler for the headers it does get.
	api := securityHeaders(cors(d.CORSOrigin, d.Authn.Middleware(mux)))
	if d.UI == nil {
		return requestLogger(recoverer(api))
	}
	return requestLogger(recoverer(splitUI(api, d.UI)))
}
