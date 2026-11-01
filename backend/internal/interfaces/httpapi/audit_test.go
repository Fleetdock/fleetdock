package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/Fleetdock/fleetdock/backend/internal/app/audit"
	authapp "github.com/Fleetdock/fleetdock/backend/internal/app/auth"
	auditdom "github.com/Fleetdock/fleetdock/backend/internal/domain/audit"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

func TestAuditRoutesExist(t *testing.T) {
	src, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{}
	for _, m := range regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+ [^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		registered[m[1]] = true
	}
	for pattern, r := range auditRoutes {
		if !registered[pattern] {
			t.Errorf("audited route %q is not registered in router.go", pattern)
		}
		if r.action == "" || r.resourceType == "" || !strings.Contains(r.action, ".") {
			t.Errorf("route %q: incomplete %+v", pattern, r)
		}
		if r.idParam != "" && !strings.Contains(pattern, "{"+r.idParam+"}") {
			t.Errorf("route %q: id param %q not in the pattern", pattern, r.idParam)
		}
	}
}

type memAudit struct{ events []auditdom.Event }

func (m *memAudit) Append(_ context.Context, e *auditdom.Event) error {
	m.events = append(m.events, *e)
	return nil
}
func (m *memAudit) List(context.Context, auditdom.Filter) ([]auditdom.Event, error) { return nil, nil }
func (m *memAudit) PruneBefore(context.Context, time.Time) (int64, error)           { return 0, nil }
func (m *memAudit) ResourceName(_ context.Context, typ string, id uuid.UUID) string {
	return typ + ":" + id.String()[:4]
}

func TestAuditMiddleware(t *testing.T) {
	repo := &memAudit{}
	svc := auditapp.NewService(repo)
	created := uuid.New()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/users", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusCreated, map[string]any{"id": created, "email": "new@x.dev", "token": "s3cret"})
	})
	mux.HandleFunc("DELETE /v1/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, apperr.Forbidden("insufficient permissions"))
	})
	mux.HandleFunc("PATCH /v1/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, apperr.Invalid("name", "too long"))
	})
	mux.HandleFunc("POST /v1/databases/{id}/query", func(w http.ResponseWriter, r *http.Request) {
		var req queryRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		auditQuery(r, req.SQL, req.Preview)
		writeJSON(w, http.StatusOK, map[string]any{})
	})
	mux.HandleFunc("POST /v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		auditapp.SetActor(r.Context(), nil, "who@x.dev")
		auditapp.Fail(r.Context())
		writeError(w, apperr.Unauthorized("invalid email or password"))
	})
	mux.HandleFunc("POST /v1/databases/{id}/credentials", func(w http.ResponseWriter, r *http.Request) {
		id := uuid.MustParse(r.PathValue("id"))
		_ = svc.Record(r.Context(), nil, "credential.create", "database", &id, map[string]any{"name": "app"})
		writeJSON(w, http.StatusCreated, map[string]any{})
	})
	mux.HandleFunc("GET /v1/users", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, nil) })

	al := &auditLog{svc: svc, mux: mux, routes: auditRoutes}
	uid := uuid.New()
	tok := uuid.New()
	withCaller := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/auth/login" {
				r = r.WithContext(withPrincipal(r.Context(), &authapp.Principal{UserID: uid, Email: "admin@x.dev", TokenID: &tok}))
			}
			next.ServeHTTP(w, r)
		})
	}
	h := withCaller(al.Handler(mux))
	do := func(method, path, body string) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.RemoteAddr = "192.0.2.7:5555"
		req.Header.Set("User-Agent", "test-agent")
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	dbID := uuid.New()
	victim := uuid.New()

	do("POST", "/v1/users", `{}`)
	do("DELETE", "/v1/users/"+victim.String(), "")
	do("PATCH", "/v1/users/"+victim.String(), `{}`)                           // 422: not logged
	do("GET", "/v1/users", "")                                                // not audited
	do("POST", "/v1/databases/"+dbID.String()+"/query", `{"sql":"SELECT 1"}`) // read: skipped
	do("POST", "/v1/databases/"+dbID.String()+"/query", `{"sql":"UPDATE t SET a=1; SELECT 1","preview":true}`)
	do("POST", "/v1/databases/"+dbID.String()+"/query", `{"sql":"UPDATE t SET a=1; SELECT 1"}`)
	do("POST", "/v1/auth/login", `{}`)
	do("POST", "/v1/databases/"+dbID.String()+"/credentials", `{}`)

	if len(repo.events) != 5 {
		for _, e := range repo.events {
			t.Logf("%s %s", e.Action, e.Outcome)
		}
		t.Fatalf("got %d events, want 5", len(repo.events))
	}
	create, del, sql, login, cred := repo.events[0], repo.events[1], repo.events[2], repo.events[3], repo.events[4]

	if create.Action != "user.create" || create.Outcome != "success" || *create.ResourceID != created ||
		create.ResourceName != "new@x.dev" || *create.ActorUserID != uid || create.ActorKind != "api_token" ||
		*create.TokenID != tok || create.IP != "192.0.2.7" || create.UserAgent != "test-agent" {
		t.Errorf("create = %+v", create)
	}
	if strings.Contains(string(create.Metadata), "s3cret") {
		t.Errorf("secret leaked into metadata: %s", create.Metadata)
	}
	if del.Action != "user.delete" || del.Outcome != "denied" || *del.ResourceID != victim || del.ResourceName != "user:"+victim.String()[:4] {
		t.Errorf("delete = %+v", del)
	}
	var meta map[string]any
	_ = json.Unmarshal(sql.Metadata, &meta)
	if sql.Action != "sql.execute" || *sql.ResourceID != dbID || meta["sql"] != "UPDATE t SET a=1; SELECT 1" ||
		meta["statements"] != float64(2) || meta["writes"] != float64(1) {
		t.Errorf("sql = %+v %v", sql, meta)
	}
	if login.Action != "auth.login" || login.Outcome != "failure" || login.ActorKind != "anonymous" ||
		login.ActorEmail != "who@x.dev" || login.ActorUserID != nil {
		t.Errorf("login = %+v", login)
	}
	if cred.Action != "credential.create" || cred.IP != "192.0.2.7" || cred.ActorEmail != "admin@x.dev" {
		t.Errorf("service event = %+v", cred)
	}
}

func TestPathParams(t *testing.T) {
	got := pathParams("POST /v1/databases/{id}/tables/{table}/rows", "/v1/databases/abc/tables/public.users/rows")
	if got["id"] != "abc" || got["table"] != "public.users" || len(got) != 2 {
		t.Errorf("params = %v", got)
	}
	if len(pathParams("GET /v1/x/{id}", "/v1/x/a/b")) != 0 {
		t.Error("length mismatch should not match")
	}
}

func TestCSVSafe(t *testing.T) {
	for in, want := range map[string]string{"=1+1": "'=1+1", "@cmd": "'@cmd", "ok": "ok", "": ""} {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q", in, got)
		}
	}
}
