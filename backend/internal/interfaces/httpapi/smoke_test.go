package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	authapp "github.com/TajBrains/fleetdock/backend/internal/app/auth"
	userdom "github.com/TajBrains/fleetdock/backend/internal/domain/user"
	"github.com/TajBrains/fleetdock/backend/internal/openapi"
	"github.com/TajBrains/fleetdock/backend/internal/platform/auth"
)

// newSmokeHandler wires public routes and auth login for HTTP smoke tests.
func newSmokeHandler(t *testing.T) http.Handler {
	t.Helper()

	id := uuid.New()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	users := &stubAuthRepo{
		users: map[uuid.UUID]userdom.User{
			id: {ID: id, Email: "admin@example.com", Status: "active"},
		},
		perms: map[uuid.UUID][]string{id: {"owner"}},
		creds: map[string]userdom.Credentials{
			"admin@example.com": {
				User: userdom.User{ID: id, Email: "admin@example.com", Status: "active"},
				Hash: hash,
			},
		},
	}

	authSvc := authapp.NewService(users, nil, auth.NewJWT("smoke-secret", time.Hour))
	authHandler := NewAuthHandler(authSvc)
	authn := NewAuthenticator(authSvc)
	install := NewInstallHandler("https://cp.example.com", t.TempDir())
	docs := NewDocsHandler()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /openapi.yaml", docs.Spec)
	mux.HandleFunc("GET /install.sh", install.Script)
	mux.HandleFunc("POST /v1/auth/login", authHandler.Login)
	mux.HandleFunc("GET /v1/auth/me", authn.Middleware(http.HandlerFunc(authHandler.Me)).ServeHTTP)

	return authn.Middleware(mux)
}

func TestSmoke_Healthz(t *testing.T) {
	h := newSmokeHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestSmoke_Readyz(t *testing.T) {
	h := newSmokeHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestSmoke_OpenAPISpec(t *testing.T) {
	h := newSmokeHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "yaml") {
		t.Fatalf("expected yaml content-type, got %q", rec.Header().Get("Content-Type"))
	}
	if len(rec.Body.Bytes()) < len(openapi.Spec)/2 {
		t.Fatal("openapi response looks truncated")
	}
	if !strings.Contains(rec.Body.String(), "openapi:") {
		t.Fatal("expected openapi document body")
	}
}

func TestSmoke_InstallScript(t *testing.T) {
	h := newSmokeHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "https://cp.example.com") {
		t.Fatalf("install script missing public URL")
	}
	if !strings.Contains(body, "FLEETDOCK_TOKEN") {
		t.Fatal("install script missing token env var")
	}
}

func TestSmoke_LoginAndMe(t *testing.T) {
	h := newSmokeHandler(t)

	loginBody, _ := json.Marshal(map[string]string{
		"email":    "admin@example.com",
		"password": "secret123",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(loginBody))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var loginResp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatal(err)
	}
	if loginResp.Token == "" {
		t.Fatal("expected JWT token")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	meReq = meReq.WithContext(context.Background())
	meReq.Header.Set("Authorization", "Bearer "+loginResp.Token)
	meRec := httptest.NewRecorder()
	h.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusOK {
		t.Fatalf("/auth/me: expected 200, got %d (%s)", meRec.Code, meRec.Body.String())
	}
	var me meResponse
	if err := json.Unmarshal(meRec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.Email != "admin@example.com" {
		t.Fatalf("unexpected user: %+v", me)
	}
}

func TestSmoke_Login_InvalidPassword(t *testing.T) {
	h := newSmokeHandler(t)
	body, _ := json.Marshal(map[string]string{
		"email":    "admin@example.com",
		"password": "wrong",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
