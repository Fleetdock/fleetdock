package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	dbadminapp "github.com/TajBrains/fleetdock/backend/internal/app/dbadmin"
	"github.com/TajBrains/fleetdock/backend/internal/platform/engine"
)

// DBAdminHandler exposes live database administration: database accounts,
// grants, tables and data browsing.
type DBAdminHandler struct {
	svc *dbadminapp.Service
}

// NewDBAdminHandler builds the handler.
func NewDBAdminHandler(svc *dbadminapp.Service) *DBAdminHandler { return &DBAdminHandler{svc: svc} }

// ---- Instance-level ----

// ListDBUsers handles GET /v1/instances/{id}/db-users.
func (h *DBAdminHandler) ListDBUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.ListDBUsers(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if users == nil {
		users = []engine.DBUser{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

type dbUserRequest struct {
	Username string `json:"username"`
	Host     string `json:"host"`
	Password string `json:"password"`
}

// CreateDBUser handles POST /v1/instances/{id}/db-users.
func (h *DBAdminHandler) CreateDBUser(w http.ResponseWriter, r *http.Request) {
	var req dbUserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.CreateDBUser(r.Context(), r.PathValue("id"), req.Username, req.Host, req.Password); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

// DropDBUser handles POST /v1/instances/{id}/db-users/drop.
// (POST body instead of DELETE path segments: hosts like '%' don't survive URLs well.)
func (h *DBAdminHandler) DropDBUser(w http.ResponseWriter, r *http.Request) {
	var req dbUserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.DropDBUser(r.Context(), r.PathValue("id"), req.Username, req.Host); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// UserGrants handles GET /v1/instances/{id}/db-users/grants?username=&host=.
func (h *DBAdminHandler) UserGrants(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	grants, err := h.svc.UserGrants(r.Context(), r.PathValue("id"), q.Get("username"), q.Get("host"))
	if err != nil {
		writeError(w, err)
		return
	}
	if grants == nil {
		grants = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": grants})
}

type grantRequest struct {
	Username   string   `json:"username"`
	Host       string   `json:"host"`
	Database   string   `json:"database"`
	Privileges []string `json:"privileges"`
}

// Grant handles POST /v1/instances/{id}/grants.
func (h *DBAdminHandler) Grant(w http.ResponseWriter, r *http.Request) {
	var req grantRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.Grant(r.Context(), r.PathValue("id"), req.Username, req.Host, req.Database, req.Privileges); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Revoke handles POST /v1/instances/{id}/grants/revoke.
func (h *DBAdminHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	var req grantRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.Revoke(r.Context(), r.PathValue("id"), req.Username, req.Host, req.Database); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---- Database-level ----

// SchemaGrants handles GET /v1/databases/{id}/grants.
func (h *DBAdminHandler) SchemaGrants(w http.ResponseWriter, r *http.Request) {
	grants, err := h.svc.SchemaGrants(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if grants == nil {
		grants = []engine.SchemaGrant{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": grants})
}

// GrantOnDatabase handles POST /v1/databases/{id}/grants.
func (h *DBAdminHandler) GrantOnDatabase(w http.ResponseWriter, r *http.Request) {
	var req grantRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.GrantOnDatabase(r.Context(), r.PathValue("id"), req.Username, req.Host, req.Privileges); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RevokeOnDatabase handles POST /v1/databases/{id}/grants/revoke.
func (h *DBAdminHandler) RevokeOnDatabase(w http.ResponseWriter, r *http.Request) {
	var req grantRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.RevokeOnDatabase(r.Context(), r.PathValue("id"), req.Username, req.Host); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListDBUsersForDatabase handles GET /v1/databases/{id}/db-users.
func (h *DBAdminHandler) ListDBUsersForDatabase(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.ListDBUsersForDatabase(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if users == nil {
		users = []engine.DBUser{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

// ListTables handles GET /v1/databases/{id}/tables.
func (h *DBAdminHandler) ListTables(w http.ResponseWriter, r *http.Request) {
	tables, err := h.svc.ListTables(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if tables == nil {
		tables = []engine.TableInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tables})
}

// TableRows handles GET /v1/databases/{id}/tables/{table}/rows.
func (h *DBAdminHandler) TableRows(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := h.svc.TableRows(r.Context(), r.PathValue("id"), r.PathValue("table"),
		atoiDefault(q.Get("limit"), 50), atoiDefault(q.Get("offset"), 0))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// ListPrivileges handles GET /v1/db-privileges (the grantable catalog).
func (h *DBAdminHandler) ListPrivileges(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": engine.GrantablePrivileges})
}

// TableSchema handles GET /v1/databases/{id}/tables/{table}/schema.
func (h *DBAdminHandler) TableSchema(w http.ResponseWriter, r *http.Request) {
	schema, err := h.svc.TableSchema(r.Context(), r.PathValue("id"), r.PathValue("table"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, schema)
}

type queryRequest struct {
	SQL   string `json:"sql"`
	Limit int    `json:"limit"`
}

// Query handles POST /v1/databases/{id}/query. Whether write statements are
// allowed is derived from the caller's database:write permission, so a
// read-only user can open the console but only run reads.
func (h *DBAdminHandler) Query(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	allowWrite := principalFrom(r.Context()).Can("database:write")
	res, err := h.svc.Query(r.Context(), r.PathValue("id"), req.SQL, req.Limit, allowWrite)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ExportTable handles GET /v1/databases/{id}/tables/{table}/export, streaming
// the whole table as CSV.
func (h *DBAdminHandler) ExportTable(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")
	h.streamCSV(w, csvFilename(table), func(onStart func()) (int64, error) {
		return h.svc.ExportTableCSV(r.Context(), r.PathValue("id"), table, w, onStart)
	})
}

// ExportQuery handles POST /v1/databases/{id}/export, streaming a read-only
// query's result set as CSV.
func (h *DBAdminHandler) ExportQuery(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	h.streamCSV(w, "query.csv", func(onStart func()) (int64, error) {
		return h.svc.ExportQueryCSV(r.Context(), r.PathValue("id"), req.SQL, w, onStart)
	})
}

// streamCSV runs a CSV export, sending a clean JSON error if it fails before any
// byte is written and otherwise setting download headers on the success path.
// An error after streaming has begun can only be logged (status is already 200).
func (h *DBAdminHandler) streamCSV(w http.ResponseWriter, filename string, run func(onStart func()) (int64, error)) {
	started := false
	_, err := run(func() {
		started = true
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		w.WriteHeader(http.StatusOK)
	})
	if err != nil {
		if !started {
			writeError(w, err)
			return
		}
		slog.Error("csv export failed mid-stream", "error", err.Error())
	}
}

// csvFilename builds a safe download filename from a table name.
func csvFilename(table string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, table)
	if safe == "" {
		safe = "table"
	}
	return safe + ".csv"
}
