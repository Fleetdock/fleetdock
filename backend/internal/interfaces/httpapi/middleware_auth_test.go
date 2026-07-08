package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	authapp "github.com/TajBrains/db-manager/backend/internal/app/auth"
	userdom "github.com/TajBrains/db-manager/backend/internal/domain/user"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
	"github.com/TajBrains/db-manager/backend/internal/platform/auth"
)

type stubAuthRepo struct {
	users map[uuid.UUID]userdom.User
	perms map[uuid.UUID][]string
}

func (r *stubAuthRepo) GetCredentialsByEmail(_ context.Context, _ string) (userdom.Credentials, error) {
	return userdom.Credentials{}, apperr.NotFound("not found")
}

func (r *stubAuthRepo) GetByID(_ context.Context, id uuid.UUID) (userdom.User, error) {
	u, ok := r.users[id]
	if !ok {
		return userdom.User{}, apperr.NotFound("not found")
	}
	return u, nil
}

func (r *stubAuthRepo) PermissionsFor(_ context.Context, id uuid.UUID) ([]string, error) {
	return r.perms[id], nil
}

func (r *stubAuthRepo) CountUsers(_ context.Context) (int, error) { return 0, nil }

func (r *stubAuthRepo) CreateWithRole(_ context.Context, _ *userdom.User, _ string, _ string) error {
	return nil
}

func TestAuthenticator_PublicRoutes(t *testing.T) {
	id := uuid.New()
	jwt := auth.NewJWT("secret", time.Hour)
	users := &stubAuthRepo{
		users: map[uuid.UUID]userdom.User{id: {ID: id, Email: "u@example.com", Status: "active"}},
		perms: map[uuid.UUID][]string{id: {"server:read"}},
	}
	svc := authapp.NewService(users, nil, jwt)
	authn := NewAuthenticator(svc)

	called := false
	handler := authn.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/healthz", "/v1/auth/login", "/install.sh", "/agent/v1/heartbeat"} {
		called = false
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if !called {
			t.Fatalf("expected public route %s to pass through", path)
		}
	}
}

func TestAuthenticator_RequiresBearerToken(t *testing.T) {
	svc := authapp.NewService(&stubAuthRepo{}, nil, auth.NewJWT("secret", time.Hour))
	authn := NewAuthenticator(svc)
	handler := authn.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/servers", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequirePerm_ForbiddenWithoutPermission(t *testing.T) {
	handler := requirePerm("server:write", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/servers", nil)
	req = req.WithContext(withPrincipal(req.Context(), authapp.NewPrincipal(uuid.New(), "u@example.com", "server:read")))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRequirePerm_AllowsWithPermission(t *testing.T) {
	handler := requirePerm("server:read", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/servers", nil)
	req = req.WithContext(withPrincipal(req.Context(), authapp.NewPrincipal(uuid.New(), "u@example.com", "server:read")))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
