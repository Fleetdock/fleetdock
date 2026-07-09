// Package authapp holds authentication/authorization use cases: password
// login issuing JWTs, resolving a request principal (from a JWT or an API
// token), and one-time admin bootstrap.
package authapp

import (
	"context"
	"strings"

	"github.com/google/uuid"

	authz "github.com/TajBrains/db-manager/backend/internal/domain/authz"
	tokendom "github.com/TajBrains/db-manager/backend/internal/domain/token"
	userdom "github.com/TajBrains/db-manager/backend/internal/domain/user"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
	"github.com/TajBrains/db-manager/backend/internal/platform/auth"
)

// apiTokenPrefix marks a presented credential as an API token rather than a JWT.
const apiTokenPrefix = "mdcp_"

// Principal is the authenticated caller plus its effective (scoped) grants.
type Principal struct {
	UserID uuid.UUID
	Email  string
	grants []authz.Grant
}

// Can reports whether the principal holds the given permission at global scope.
// Use CanOn for resource-scoped checks.
func (p *Principal) Can(perm string) bool {
	if p == nil {
		return false
	}
	return authz.HasGlobal(p.grants, perm)
}

// CanOn reports whether the principal holds perm on a resource with the given
// ancestry (a global grant of perm always allows).
func (p *Principal) CanOn(perm string, anc authz.Ancestry) bool {
	if p == nil {
		return false
	}
	return authz.Allow(p.grants, perm, anc)
}

// AllPermissions returns the distinct permissions the principal holds at any
// scope (global or scoped). Used to bound the scopes a user may mint on a token.
func (p *Principal) AllPermissions() []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, g := range p.grants {
		if _, ok := seen[g.Permission]; ok {
			continue
		}
		seen[g.Permission] = struct{}{}
		out = append(out, g.Permission)
	}
	return out
}

// CanAny reports whether the principal holds perm at any scope (global or
// scoped). Used to gate list endpoints, which then filter their results.
func (p *Principal) CanAny(perm string) bool {
	if p == nil {
		return false
	}
	for _, g := range p.grants {
		if g.Permission == perm {
			return true
		}
	}
	return false
}

// Readable returns which resources the principal may read for perm.
func (p *Principal) Readable(perm string) authz.ReadSet {
	if p == nil {
		return authz.ReadSet{}
	}
	return authz.ReadableScope(p.grants, perm)
}

// Grants returns a copy of the principal's scoped grants.
func (p *Principal) Grants() []authz.Grant {
	if p == nil {
		return nil
	}
	out := make([]authz.Grant, len(p.grants))
	copy(out, p.grants)
	return out
}

// NewPrincipal builds a principal with global grants directly (used by tests
// and internal wiring).
func NewPrincipal(userID uuid.UUID, email string, perms ...string) *Principal {
	g := make([]authz.Grant, 0, len(perms))
	for _, pm := range perms {
		g = append(g, authz.Grant{Permission: pm, Scope: authz.Scope{Type: authz.ScopeGlobal}})
	}
	return &Principal{UserID: userID, Email: email, grants: g}
}

// NewPrincipalWithGrants builds a principal from explicit scoped grants (used
// by tests and internal wiring that needs scope-aware principals).
func NewPrincipalWithGrants(userID uuid.UUID, email string, grants []authz.Grant) *Principal {
	return &Principal{UserID: userID, Email: email, grants: grants}
}

// Permissions returns the distinct permissions the principal holds at global
// scope (used by /auth/me for global-page and nav gating).
func (p *Principal) Permissions() []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, g := range p.grants {
		if g.Scope.Type != authz.ScopeGlobal {
			continue
		}
		if _, ok := seen[g.Permission]; ok {
			continue
		}
		seen[g.Permission] = struct{}{}
		out = append(out, g.Permission)
	}
	return out
}

// Service implements auth use cases.
type Service struct {
	users  userdom.Repository
	tokens tokendom.Repository
	jwt    *auth.JWT
}

// NewService wires the auth service.
func NewService(users userdom.Repository, tokens tokendom.Repository, jwt *auth.JWT) *Service {
	return &Service{users: users, tokens: tokens, jwt: jwt}
}

// LoginResult is returned on a successful password login.
type LoginResult struct {
	Token string
	User  userdom.User
}

// Authenticate verifies an email/password and returns a signed JWT.
func (s *Service) Authenticate(ctx context.Context, email, password string) (LoginResult, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	creds, err := s.users.GetCredentialsByEmail(ctx, email)
	if err != nil {
		// Do not leak whether the email exists.
		return LoginResult{}, apperr.Unauthorized("invalid email or password")
	}
	if !auth.CheckPassword(creds.Hash, password) {
		return LoginResult{}, apperr.Unauthorized("invalid email or password")
	}
	if creds.User.Status != "active" {
		return LoginResult{}, apperr.Unauthorized("account is suspended")
	}
	tok, err := s.jwt.Issue(creds.User.ID.String())
	if err != nil {
		return LoginResult{}, apperr.Internal(err)
	}
	return LoginResult{Token: tok, User: creds.User}, nil
}

// Principal resolves a presented credential (JWT or API token) to a Principal.
func (s *Service) Principal(ctx context.Context, credential string) (*Principal, error) {
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return nil, apperr.Unauthorized("missing credential")
	}

	var userID uuid.UUID
	var tokenScopes []string // when non-empty, restrict grants to these permissions

	if strings.HasPrefix(credential, apiTokenPrefix) {
		lookup, err := s.tokens.ResolveHash(ctx, auth.HashToken(credential))
		if err != nil {
			return nil, apperr.Unauthorized("invalid or revoked token")
		}
		userID = lookup.UserID
		tokenScopes = lookup.Scopes
	} else {
		sub, err := s.jwt.Verify(credential)
		if err != nil {
			return nil, apperr.Unauthorized("invalid or expired session")
		}
		userID, err = uuid.Parse(sub)
		if err != nil {
			return nil, apperr.Unauthorized("malformed subject")
		}
	}

	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, apperr.Unauthorized("account no longer exists")
	}
	if u.Status != "active" {
		return nil, apperr.Unauthorized("account is suspended")
	}
	grants, err := s.users.GrantsFor(ctx, userID)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	// An API token restricts the user's grants to its scopes. An empty scope
	// list means the token inherits all of the user's grants (session-like).
	if len(tokenScopes) > 0 {
		grants = restrictToScopes(grants, tokenScopes)
	}
	return &Principal{UserID: u.ID, Email: u.Email, grants: grants}, nil
}

// EnsureAdmin creates a bootstrap owner account if there are no users yet.
func (s *Service) EnsureAdmin(ctx context.Context, email, password string) (bool, error) {
	n, err := s.users.CountUsers(ctx)
	if err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return false, err
	}
	u := &userdom.User{
		ID:     uuid.New(),
		Email:  strings.TrimSpace(strings.ToLower(email)),
		Name:   "Administrator",
		Status: "active",
	}
	if err := s.users.CreateWithRole(ctx, u, hash, "owner"); err != nil {
		return false, err
	}
	return true, nil
}

// restrictToScopes keeps only the grants whose permission is in scopes (an API
// token can never exceed its declared scopes).
func restrictToScopes(grants []authz.Grant, scopes []string) []authz.Grant {
	allow := make(map[string]struct{}, len(scopes))
	for _, s := range scopes {
		allow[s] = struct{}{}
	}
	out := make([]authz.Grant, 0, len(grants))
	for _, g := range grants {
		if _, ok := allow[g.Permission]; ok {
			out = append(out, g)
		}
	}
	return out
}
