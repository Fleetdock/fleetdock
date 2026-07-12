package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	authapp "github.com/Fleetdock/fleetdock/backend/internal/app/auth"
	authzapp "github.com/Fleetdock/fleetdock/backend/internal/app/authz"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	userdom "github.com/Fleetdock/fleetdock/backend/internal/domain/user"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/auth"
)

type stubAuthRepo struct {
	users map[uuid.UUID]userdom.User
	perms map[uuid.UUID][]string
	creds map[string]userdom.Credentials
}

func (r *stubAuthRepo) GetCredentialsByEmail(_ context.Context, email string) (userdom.Credentials, error) {
	if r.creds != nil {
		if c, ok := r.creds[email]; ok {
			return c, nil
		}
	}
	return userdom.Credentials{}, apperr.NotFound("not found")
}

func (r *stubAuthRepo) GetByID(_ context.Context, id uuid.UUID) (userdom.User, error) {
	u, ok := r.users[id]
	if !ok {
		return userdom.User{}, apperr.NotFound("not found")
	}
	return u, nil
}

func (r *stubAuthRepo) GrantsFor(_ context.Context, id uuid.UUID) ([]authz.Grant, error) {
	out := make([]authz.Grant, 0, len(r.perms[id]))
	for _, p := range r.perms[id] {
		out = append(out, authz.Grant{Permission: p, Scope: authz.Scope{Type: authz.ScopeGlobal}})
	}
	return out, nil
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

	for _, path := range []string{"/healthz", "/readyz", "/v1/auth/login", "/install.sh", "/agent/v1/heartbeat"} {
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

// fakeAuthzRepo resolves every instance to a fixed server, for scope tests.
type fakeAuthzRepo struct{ serverID uuid.UUID }

func (r fakeAuthzRepo) ServerOfInstance(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	return r.serverID, nil
}
func (r fakeAuthzRepo) LineageOfDatabase(_ context.Context, _ uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	return uuid.New(), r.serverID, nil
}
func (r fakeAuthzRepo) DatabaseOfBackup(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	return uuid.New(), nil
}
func (r fakeAuthzRepo) JobResource(_ context.Context, _ uuid.UUID) (string, uuid.UUID, error) {
	return "", uuid.Nil, nil
}

func newScopedPrincipal(perm string, scope authz.Scope) *authapp.Principal {
	// Build a principal carrying a single scoped grant via a token-like path is
	// awkward; NewPrincipalWithGrants keeps the test focused on the middleware.
	return authapp.NewPrincipalWithGrants(uuid.New(), "u@example.com",
		[]authz.Grant{{Permission: perm, Scope: scope}})
}

func TestRequireResourcePerm_ScopeEnforced(t *testing.T) {
	serverA := uuid.New()
	resolver := authzapp.NewResolver(fakeAuthzRepo{serverID: serverA})
	instanceID := uuid.New()

	handler := requireResourcePerm(resolver, "instance:write", authz.ResourceInstance, "id",
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	// In-scope: grant on serverA covers the instance on serverA.
	req := httptest.NewRequest(http.MethodPost, "/v1/instances/"+instanceID.String()+"/start", nil)
	req.SetPathValue("id", instanceID.String())
	req = req.WithContext(withPrincipal(req.Context(),
		newScopedPrincipal("instance:write", authz.Scope{Type: authz.ScopeServer, ID: serverA})))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("in-scope: expected 200, got %d", rec.Code)
	}

	// Out-of-scope: grant on a different server must be forbidden.
	req = httptest.NewRequest(http.MethodPost, "/v1/instances/"+instanceID.String()+"/start", nil)
	req.SetPathValue("id", instanceID.String())
	req = req.WithContext(withPrincipal(req.Context(),
		newScopedPrincipal("instance:write", authz.Scope{Type: authz.ScopeServer, ID: uuid.New()})))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("out-of-scope: expected 403, got %d", rec.Code)
	}
}
