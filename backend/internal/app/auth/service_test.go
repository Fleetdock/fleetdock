package authapp

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	tokendom "github.com/Fleetdock/fleetdock/backend/internal/domain/token"
	userdom "github.com/Fleetdock/fleetdock/backend/internal/domain/user"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/auth"
)

type fakeUserRepo struct {
	creds  map[string]userdom.Credentials
	users  map[uuid.UUID]userdom.User
	perms  map[uuid.UUID][]string      // global permissions
	grants map[uuid.UUID][]authz.Grant // scoped grants (overrides perms when set)
	count  int
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		creds:  map[string]userdom.Credentials{},
		users:  map[uuid.UUID]userdom.User{},
		perms:  map[uuid.UUID][]string{},
		grants: map[uuid.UUID][]authz.Grant{},
	}
}

func (r *fakeUserRepo) GetCredentialsByEmail(_ context.Context, email string) (userdom.Credentials, error) {
	c, ok := r.creds[email]
	if !ok {
		return userdom.Credentials{}, apperr.NotFound("user not found")
	}
	return c, nil
}

func (r *fakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (userdom.User, error) {
	u, ok := r.users[id]
	if !ok {
		return userdom.User{}, apperr.NotFound("user not found")
	}
	return u, nil
}

func (r *fakeUserRepo) GrantsFor(_ context.Context, id uuid.UUID) ([]authz.Grant, error) {
	if g, ok := r.grants[id]; ok && len(g) > 0 {
		return g, nil
	}
	out := make([]authz.Grant, 0, len(r.perms[id]))
	for _, p := range r.perms[id] {
		out = append(out, authz.Grant{Permission: p, Scope: authz.Scope{Type: authz.ScopeGlobal}})
	}
	return out, nil
}

func (r *fakeUserRepo) CountUsers(_ context.Context) (int, error) {
	return r.count, nil
}

func (r *fakeUserRepo) CreateWithRole(_ context.Context, u *userdom.User, _ string, roleName string) error {
	r.users[u.ID] = *u
	r.creds[u.Email] = userdom.Credentials{User: *u, Hash: "unused"}
	r.perms[u.ID] = []string{"owner"}
	r.count++
	return nil
}

type fakeTokenRepo struct {
	lookups map[string]tokendom.Lookup
}

func (r *fakeTokenRepo) Create(_ context.Context, _ *tokendom.Token, _ string) error { return nil }
func (r *fakeTokenRepo) ListByUser(_ context.Context, _ uuid.UUID) ([]*tokendom.Token, error) {
	return nil, nil
}
func (r *fakeTokenRepo) Revoke(_ context.Context, _, _ uuid.UUID) error { return nil }
func (r *fakeTokenRepo) ResolveHash(_ context.Context, hash string) (tokendom.Lookup, error) {
	l, ok := r.lookups[hash]
	if !ok {
		return tokendom.Lookup{}, apperr.NotFound("token not found")
	}
	return l, nil
}

func TestAuthenticate_Success(t *testing.T) {
	id := uuid.New()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	users := newFakeUserRepo()
	users.creds["admin@example.com"] = userdom.Credentials{
		User: userdom.User{ID: id, Email: "admin@example.com", Status: "active"},
		Hash: hash,
	}
	svc := NewService(users, &fakeTokenRepo{}, auth.NewJWT("test-secret", time.Hour))

	res, err := svc.Authenticate(context.Background(), "admin@example.com", "secret123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Token == "" {
		t.Fatal("expected token")
	}
	if res.User.ID != id {
		t.Fatalf("expected user id %s, got %s", id, res.User.ID)
	}
}

func TestAuthenticate_InvalidPassword(t *testing.T) {
	id := uuid.New()
	hash, _ := auth.HashPassword("secret123")
	users := newFakeUserRepo()
	users.creds["admin@example.com"] = userdom.Credentials{
		User: userdom.User{ID: id, Email: "admin@example.com", Status: "active"},
		Hash: hash,
	}
	svc := NewService(users, &fakeTokenRepo{}, auth.NewJWT("test-secret", time.Hour))

	_, err := svc.Authenticate(context.Background(), "admin@example.com", "wrong")
	if apperr.KindOf(err) != apperr.KindUnauthorized {
		t.Fatalf("expected unauthorized, got %v", apperr.KindOf(err))
	}
}

func TestAuthenticate_SuspendedUser(t *testing.T) {
	id := uuid.New()
	hash, _ := auth.HashPassword("secret123")
	users := newFakeUserRepo()
	users.creds["admin@example.com"] = userdom.Credentials{
		User: userdom.User{ID: id, Email: "admin@example.com", Status: "suspended"},
		Hash: hash,
	}
	svc := NewService(users, &fakeTokenRepo{}, auth.NewJWT("test-secret", time.Hour))

	_, err := svc.Authenticate(context.Background(), "admin@example.com", "secret123")
	if apperr.KindOf(err) != apperr.KindUnauthorized {
		t.Fatalf("expected unauthorized, got %v", apperr.KindOf(err))
	}
}

func TestPrincipal_JWT(t *testing.T) {
	id := uuid.New()
	jwt := auth.NewJWT("test-secret", time.Hour)
	tok, err := jwt.Issue(id.String(), 0)
	if err != nil {
		t.Fatal(err)
	}
	users := newFakeUserRepo()
	users.users[id] = userdom.User{ID: id, Email: "u@example.com", Status: "active"}
	users.perms[id] = []string{"server:read"}
	svc := NewService(users, &fakeTokenRepo{}, jwt)

	p, err := svc.Principal(context.Background(), tok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.Can("server:read") {
		t.Fatal("expected server:read permission")
	}
}

func TestPrincipal_JWTStaleEpochRejected(t *testing.T) {
	id := uuid.New()
	jwt := auth.NewJWT("test-secret", time.Hour)
	tok, err := jwt.Issue(id.String(), 0) // issued at epoch 0
	if err != nil {
		t.Fatal(err)
	}
	users := newFakeUserRepo()
	// The user's epoch has since advanced (e.g. a password reset).
	users.users[id] = userdom.User{ID: id, Email: "u@example.com", Status: "active", TokenEpoch: 1}
	users.perms[id] = []string{"server:read"}
	svc := NewService(users, &fakeTokenRepo{}, jwt)

	if _, err := svc.Principal(context.Background(), tok); apperr.KindOf(err) != apperr.KindUnauthorized {
		t.Fatalf("expected unauthorized for stale epoch, got %v", apperr.KindOf(err))
	}
}

func TestPrincipal_TokenEmptyScopesInheritAll(t *testing.T) {
	id := uuid.New()
	users := newFakeUserRepo()
	users.users[id] = userdom.User{ID: id, Email: "u@example.com", Status: "active"}
	users.perms[id] = []string{"database:read", "database:write"}
	tokens := &fakeTokenRepo{lookups: map[string]tokendom.Lookup{
		auth.HashToken("fleetd_empty"): {UserID: id, Scopes: []string{}}, // no scopes
	}}
	svc := NewService(users, tokens, auth.NewJWT("secret", time.Hour))

	p, err := svc.Principal(context.Background(), "fleetd_empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.Can("database:read") || !p.Can("database:write") {
		t.Fatal("empty-scope token should inherit all of the user's grants")
	}
}

func TestPrincipal_TokenScopesRestrict(t *testing.T) {
	id := uuid.New()
	users := newFakeUserRepo()
	users.users[id] = userdom.User{ID: id, Email: "u@example.com", Status: "active"}
	users.perms[id] = []string{"database:read", "database:write"}
	tokens := &fakeTokenRepo{lookups: map[string]tokendom.Lookup{
		auth.HashToken("fleetd_ro"): {UserID: id, Scopes: []string{"database:read"}},
	}}
	svc := NewService(users, tokens, auth.NewJWT("secret", time.Hour))

	p, err := svc.Principal(context.Background(), "fleetd_ro")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.Can("database:read") {
		t.Fatal("scoped token should keep database:read")
	}
	if p.Can("database:write") {
		t.Fatal("scoped token must not grant database:write")
	}
}

func TestPrincipal_ScopedGrantsCanOn(t *testing.T) {
	id := uuid.New()
	serverA := uuid.New()
	jwt := auth.NewJWT("secret", time.Hour)
	tok, _ := jwt.Issue(id.String(), 0)

	users := newFakeUserRepo()
	users.users[id] = userdom.User{ID: id, Email: "u@example.com", Status: "active"}
	users.grants[id] = []authz.Grant{
		{Permission: "instance:write", Scope: authz.Scope{Type: authz.ScopeServer, ID: serverA}},
	}
	svc := NewService(users, &fakeTokenRepo{}, jwt)

	p, err := svc.Principal(context.Background(), tok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Can("instance:write") {
		t.Fatal("scoped grant must not satisfy a global Can check")
	}
	if !p.CanAny("instance:write") {
		t.Fatal("CanAny should be true for a scoped grant")
	}
	inScope := authz.Ancestry{Covers: []authz.Scope{{Type: authz.ScopeServer, ID: serverA}}}
	if !p.CanOn("instance:write", inScope) {
		t.Fatal("CanOn should allow a resource under the granted server")
	}
	outScope := authz.Ancestry{Covers: []authz.Scope{{Type: authz.ScopeServer, ID: uuid.New()}}}
	if p.CanOn("instance:write", outScope) {
		t.Fatal("CanOn should deny a resource under a different server")
	}
}

func TestEnsureAdmin_OnlyWhenEmpty(t *testing.T) {
	users := newFakeUserRepo()
	svc := NewService(users, &fakeTokenRepo{}, auth.NewJWT("test-secret", time.Hour))

	created, err := svc.EnsureAdmin(context.Background(), "admin@example.com", "bootstrap-pass")
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected admin to be created")
	}

	created, err = svc.EnsureAdmin(context.Background(), "other@example.com", "other-pass")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("expected second bootstrap to be skipped")
	}
}
