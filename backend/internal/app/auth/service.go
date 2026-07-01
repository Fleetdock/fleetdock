// Package authapp holds authentication/authorization use cases: password
// login issuing JWTs, resolving a request principal (from a JWT or an API
// token), and one-time admin bootstrap.
package authapp

import (
	"context"
	"strings"

	"github.com/google/uuid"

	tokendom "github.com/mariadb-cp/db-manager/backend/internal/domain/token"
	userdom "github.com/mariadb-cp/db-manager/backend/internal/domain/user"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/auth"
)

// apiTokenPrefix marks a presented credential as an API token rather than a JWT.
const apiTokenPrefix = "mdcp_"

// Principal is the authenticated caller plus its effective permissions.
type Principal struct {
	UserID uuid.UUID
	Email  string
	perms  map[string]struct{}
}

// Can reports whether the principal holds the given permission.
func (p *Principal) Can(perm string) bool {
	if p == nil {
		return false
	}
	_, ok := p.perms[perm]
	return ok
}

// NewPrincipal builds a principal directly (used by tests and internal wiring).
func NewPrincipal(userID uuid.UUID, email string, perms ...string) *Principal {
	return &Principal{UserID: userID, Email: email, perms: toSet(perms)}
}

// Permissions returns the principal's effective permissions (sorted-agnostic).
func (p *Principal) Permissions() []string {
	out := make([]string, 0, len(p.perms))
	for k := range p.perms {
		out = append(out, k)
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
	var scoped map[string]struct{} // non-nil => restrict perms to token scopes

	if strings.HasPrefix(credential, apiTokenPrefix) {
		lookup, err := s.tokens.ResolveHash(ctx, auth.HashToken(credential))
		if err != nil {
			return nil, apperr.Unauthorized("invalid or revoked token")
		}
		userID = lookup.UserID
		scoped = toSet(lookup.Scopes)
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
	granted, err := s.users.PermissionsFor(ctx, userID)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	perms := toSet(granted)
	if scoped != nil {
		perms = intersect(perms, scoped) // API token cannot exceed its scopes
	}
	return &Principal{UserID: u.ID, Email: u.Email, perms: perms}, nil
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

func toSet(ss []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}

func intersect(a, b map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{})
	for k := range a {
		if _, ok := b[k]; ok {
			out[k] = struct{}{}
		}
	}
	return out
}
