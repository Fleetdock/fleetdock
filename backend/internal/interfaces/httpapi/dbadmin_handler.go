package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	authzapp "github.com/Fleetdock/fleetdock/backend/internal/app/authz"
	dbadminapp "github.com/Fleetdock/fleetdock/backend/internal/app/dbadmin"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// DBAdminHandler exposes live database administration: database accounts,
// grants, tables and data browsing.
type DBAdminHandler struct {
	svc      *dbadminapp.Service
	resolver *authzapp.Resolver
}

// NewDBAdminHandler builds the handler.
func NewDBAdminHandler(svc *dbadminapp.Service, resolver *authzapp.Resolver) *DBAdminHandler {
	return &DBAdminHandler{svc: svc, resolver: resolver}
}

// interactive gates console, browsing and export requests on one database. The
// route middleware has already checked database:read on it; this adds:
//   - system databases (mysql, sys, postgres) hold account definitions and
//     password hashes, so they additionally require instance:write — the
//     right to administer the instance's accounts anyway;
//   - whether the caller may write, evaluated on this database's ancestry
//     (a grant scoped to the database or its server counts, not only a
//     global one).
func (h *DBAdminHandler) interactive(r *http.Request) (allowWrite bool, err error) {
	ctx := r.Context()
	dbID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return false, apperr.Invalid("id", "must be a valid UUID")
	}
	instID, system, err := h.svc.SystemDatabaseInstance(ctx, dbID.String())
	if err != nil {
		return false, err
	}
	if system {
		if err := authorizeResource(ctx, h.resolver, "instance:write", authz.ResourceInstance, instID); err != nil {
			return false, apperr.Forbidden("system databases can only be opened by instance administrators")
		}
	}
	return authorizeResource(ctx, h.resolver, "database:write", authz.ResourceDatabase, dbID) == nil, nil
}

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
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
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
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
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
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
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
	// QueryID (optional, client-generated) lets the caller cancel the run.
	QueryID string `json:"query_id"`
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
	allowWrite, err := h.interactive(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if req.QueryID != "" {
		if _, err := uuid.Parse(req.QueryID); err != nil {
			writeError(w, apperr.Invalid("query_id", "query_id must be a UUID"))
			return
		}
	}
	in := dbadminapp.QueryInput{
		DatabaseID: r.PathValue("id"), SQL: req.SQL, Limit: req.Limit,
		AllowWrite: allowWrite, QueryID: req.QueryID,
	}
	if id := callerID(r); id != nil {
		in.UserID = *id
	}
	res, err := h.svc.Query(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// CancelQuery handles POST /v1/databases/{id}/query/{qid}/cancel. Only the
// user who started the query can cancel it.
func (h *DBAdminHandler) CancelQuery(w http.ResponseWriter, r *http.Request) {
	id := callerID(r)
	if id == nil {
		writeError(w, apperr.Unauthorized("authentication required"))
		return
	}
	if err := h.svc.CancelQuery(r.Context(), r.PathValue("id"), r.PathValue("qid"), *id); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "cancelling"})
}

// ExportTable handles GET /v1/databases/{id}/tables/{table}/export, streaming
// the whole table as CSV.
func (h *DBAdminHandler) ExportTable(w http.ResponseWriter, r *http.Request) {
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
	table := r.PathValue("table")
	h.streamCSV(w, csvFilename(table), func(onStart func()) (int64, error) {
		return h.svc.ExportTableCSV(r.Context(), r.PathValue("id"), table, w, onStart)
	})
}

// ExportQuery handles POST /v1/databases/{id}/export, streaming a read-only
// query's result set as CSV.
func (h *DBAdminHandler) ExportQuery(w http.ResponseWriter, r *http.Request) {
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
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
	// The server's WriteTimeout is sized for JSON requests; an export may
	// legitimately stream for minutes, and would otherwise be cut off mid-file
	// after a 200 had already been sent.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(exportWriteDeadline)); err != nil {
		slog.Warn("csv export: cannot extend write deadline", "error", err.Error())
	}
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

// exportWriteDeadline bounds a streamed export; a little above the service's
// export timeout so the query, not the socket, is what gives up first.
const exportWriteDeadline = 6 * time.Minute

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

// Processes handles GET /v1/instances/{id}/processes.
func (h *DBAdminHandler) Processes(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Processes(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type killProcessRequest struct {
	// Connection terminates the whole session instead of only cancelling
	// its current statement.
	Connection bool `json:"connection"`
}

// KillProcess handles POST /v1/instances/{id}/processes/{pid}/kill.
func (h *DBAdminHandler) KillProcess(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil || pid <= 0 {
		writeError(w, apperr.Invalid("pid", "must be a positive integer"))
		return
	}
	var req killProcessRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, err)
			return
		}
	}
	if err := h.svc.KillProcess(r.Context(), r.PathValue("id"), pid, req.Connection); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ServerStatus handles GET /v1/instances/{id}/status.
func (h *DBAdminHandler) ServerStatus(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ServerStatus(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Variables handles GET /v1/instances/{id}/variables.
func (h *DBAdminHandler) Variables(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Variables(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// BrowseRows handles POST /v1/databases/{id}/tables/{table}/browse (POST
// because filters and sorting travel in the body).
func (h *DBAdminHandler) BrowseRows(w http.ResponseWriter, r *http.Request) {
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
	var req engine.BrowseRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, err)
			return
		}
	}
	req.Table = r.PathValue("table")
	res, err := h.svc.BrowseRows(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type rowRequest struct {
	Key    engine.RowValues `json:"key"`
	Values engine.RowValues `json:"values"`
}

// editable gates single-row edits: interactive access plus write permission
// on this database.
func (h *DBAdminHandler) editable(r *http.Request) error {
	allowWrite, err := h.interactive(r)
	if err != nil {
		return err
	}
	if !allowWrite {
		return apperr.Forbidden("editing data requires database:write")
	}
	return nil
}

// InsertRow handles POST /v1/databases/{id}/tables/{table}/rows.
func (h *DBAdminHandler) InsertRow(w http.ResponseWriter, r *http.Request) {
	if err := h.editable(r); err != nil {
		writeError(w, err)
		return
	}
	var req rowRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.InsertRow(r.Context(), r.PathValue("id"), r.PathValue("table"), req.Values); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "inserted"})
}

// UpdateRow handles PATCH /v1/databases/{id}/tables/{table}/rows.
func (h *DBAdminHandler) UpdateRow(w http.ResponseWriter, r *http.Request) {
	if err := h.editable(r); err != nil {
		writeError(w, err)
		return
	}
	var req rowRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.UpdateRow(r.Context(), r.PathValue("id"), r.PathValue("table"), req.Key, req.Values); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteRow handles POST /v1/databases/{id}/tables/{table}/rows/delete.
func (h *DBAdminHandler) DeleteRow(w http.ResponseWriter, r *http.Request) {
	if err := h.editable(r); err != nil {
		writeError(w, err)
		return
	}
	var req rowRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.DeleteRow(r.Context(), r.PathValue("id"), r.PathValue("table"), req.Key); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// maxImportBytes caps a CSV upload.
const maxImportBytes = 100 << 20

// ImportCSV handles POST /v1/databases/{id}/tables/{table}/import with the raw
// CSV as the body (text/csv). The first line must name table columns.
func (h *DBAdminHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	if err := h.editable(r); err != nil {
		writeError(w, err)
		return
	}
	// Uploading and inserting a large file takes longer than a JSON request.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(11 * time.Minute))
	_ = rc.SetWriteDeadline(time.Now().Add(11 * time.Minute))
	body := http.MaxBytesReader(w, r.Body, maxImportBytes)
	opts := engine.ImportOptions{EmptyAsNull: r.URL.Query().Get("empty_as_null") == "true"}
	n, err := h.svc.ImportCSV(r.Context(), r.PathValue("id"), r.PathValue("table"), body, opts)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			err = apperr.Invalid("body", "the CSV file is larger than 100 MB")
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"imported": n})
}

// ---- Structure ----

// structural wraps a DDL handler: interactive access plus database:write,
// and the long write deadline an ALTER on a big table may need.
func (h *DBAdminHandler) structural(fn func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.editable(r); err != nil {
			writeError(w, err)
			return
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(6 * time.Minute))
		if err := fn(w, r); err != nil {
			writeError(w, err)
		}
	}
}

// CreateTable handles POST /v1/databases/{id}/tables.
func (h *DBAdminHandler) CreateTable(w http.ResponseWriter, r *http.Request) {
	h.structural(func(w http.ResponseWriter, r *http.Request) error {
		var spec engine.TableSpec
		if err := decodeJSON(r, &spec); err != nil {
			return err
		}
		if err := h.svc.CreateTable(r.Context(), r.PathValue("id"), spec); err != nil {
			return err
		}
		writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
		return nil
	})(w, r)
}

type alterTableRequest struct {
	Ops []engine.AlterOp `json:"ops"`
}

// AlterTable handles PATCH /v1/databases/{id}/tables/{table}.
func (h *DBAdminHandler) AlterTable(w http.ResponseWriter, r *http.Request) {
	h.structural(func(w http.ResponseWriter, r *http.Request) error {
		var req alterTableRequest
		if err := decodeJSON(r, &req); err != nil {
			return err
		}
		if err := h.svc.AlterTable(r.Context(), r.PathValue("id"), r.PathValue("table"), req.Ops); err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "altered"})
		return nil
	})(w, r)
}

// DropTable handles POST /v1/databases/{id}/tables/{table}/drop.
func (h *DBAdminHandler) DropTable(w http.ResponseWriter, r *http.Request) {
	h.structural(func(w http.ResponseWriter, r *http.Request) error {
		if err := h.svc.DropTable(r.Context(), r.PathValue("id"), r.PathValue("table")); err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "dropped"})
		return nil
	})(w, r)
}

// TruncateTable handles POST /v1/databases/{id}/tables/{table}/truncate.
func (h *DBAdminHandler) TruncateTable(w http.ResponseWriter, r *http.Request) {
	h.structural(func(w http.ResponseWriter, r *http.Request) error {
		if err := h.svc.TruncateTable(r.Context(), r.PathValue("id"), r.PathValue("table")); err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "truncated"})
		return nil
	})(w, r)
}

type renameTableRequest struct {
	NewName string `json:"new_name"`
}

// RenameTable handles POST /v1/databases/{id}/tables/{table}/rename.
func (h *DBAdminHandler) RenameTable(w http.ResponseWriter, r *http.Request) {
	h.structural(func(w http.ResponseWriter, r *http.Request) error {
		var req renameTableRequest
		if err := decodeJSON(r, &req); err != nil {
			return err
		}
		if err := h.svc.RenameTable(r.Context(), r.PathValue("id"), r.PathValue("table"), req.NewName); err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "renamed"})
		return nil
	})(w, r)
}

// CreateIndex handles POST /v1/databases/{id}/tables/{table}/indexes.
func (h *DBAdminHandler) CreateIndex(w http.ResponseWriter, r *http.Request) {
	h.structural(func(w http.ResponseWriter, r *http.Request) error {
		var spec engine.IndexSpec
		if err := decodeJSON(r, &spec); err != nil {
			return err
		}
		if err := h.svc.CreateIndex(r.Context(), r.PathValue("id"), r.PathValue("table"), spec); err != nil {
			return err
		}
		writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
		return nil
	})(w, r)
}

type dropIndexRequest struct {
	Name string `json:"name"`
}

// DropIndex handles POST /v1/databases/{id}/tables/{table}/indexes/drop.
func (h *DBAdminHandler) DropIndex(w http.ResponseWriter, r *http.Request) {
	h.structural(func(w http.ResponseWriter, r *http.Request) error {
		var req dropIndexRequest
		if err := decodeJSON(r, &req); err != nil {
			return err
		}
		if err := h.svc.DropIndex(r.Context(), r.PathValue("id"), r.PathValue("table"), req.Name); err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "dropped"})
		return nil
	})(w, r)
}

// ForeignKeys handles GET /v1/databases/{id}/tables/{table}/foreign-keys.
func (h *DBAdminHandler) ForeignKeys(w http.ResponseWriter, r *http.Request) {
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
	items, err := h.svc.ForeignKeys(r.Context(), r.PathValue("id"), r.PathValue("table"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Objects handles GET /v1/databases/{id}/objects.
func (h *DBAdminHandler) Objects(w http.ResponseWriter, r *http.Request) {
	if _, err := h.interactive(r); err != nil {
		writeError(w, err)
		return
	}
	items, err := h.svc.Objects(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type setPasswordRequest struct {
	Username string `json:"username"`
	Host     string `json:"host"`
	Password string `json:"password"`
}

// SetDBUserPassword handles POST /v1/instances/{id}/db-users/password.
func (h *DBAdminHandler) SetDBUserPassword(w http.ResponseWriter, r *http.Request) {
	var req setPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.SetDBUserPassword(r.Context(), r.PathValue("id"), req.Username, req.Host, req.Password); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}
