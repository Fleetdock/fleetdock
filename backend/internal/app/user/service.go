// Package userapp holds account-management use cases: administering
// operator accounts and roles, and self-service profile updates. It
// enforces the invariants that prevent lockout (there is always at least
// one active owner) and that admins cannot saw off the branch they sit on.
package userapp

import (
	"context"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	userdom "github.com/Fleetdock/fleetdock/backend/internal/domain/user"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/auth"
)

// Repo is the persistence surface this service needs.
type Repo interface {
	userdom.Repository
	userdom.AdminRepository
}

// Service implements account-management use cases.
type Service struct {
	repo Repo
}

// NewService wires the service.
func NewService(repo Repo) *Service { return &Service{repo: repo} }

const minPasswordLen = 8

// ---- Administration ----

// List returns all users with their roles.
func (s *Service) List(ctx context.Context) ([]userdom.WithRoles, error) {
	return s.repo.List(ctx)
}

// ListRoles returns the role catalog.
func (s *Service) ListRoles(ctx context.Context) ([]userdom.Role, error) {
	return s.repo.ListRoles(ctx)
}

// CreateInput describes a new operator account.
type CreateInput struct {
	Name     string
	Email    string
	Password string
	Role     string
}

// Create validates input and creates a user with one global role.
func (s *Service) Create(ctx context.Context, in CreateInput) (*userdom.WithRoles, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 120 {
		return nil, apperr.Invalid("name", "name is required and must be at most 120 characters")
	}
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return nil, err
	}
	if err := validatePassword(in.Password); err != nil {
		return nil, err
	}
	role := in.Role
	if role == "" {
		role = "viewer"
	}
	if err := s.ensureRoleExists(ctx, role); err != nil {
		return nil, err
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	u := &userdom.User{ID: uuid.New(), Email: email, Name: name, Status: userdom.StatusActive}
	if err := s.repo.CreateWithRole(ctx, u, hash, role); err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return nil, apperr.Conflict("a user with this email already exists")
		}
		return nil, err
	}
	return &userdom.WithRoles{User: *u, Roles: []string{role}}, nil
}

// UpdateInput carries admin-editable user fields (nil/empty = keep).
type UpdateInput struct {
	Name   *string
	Email  *string
	Status *string
	Role   *string
}

// Update edits a user account; actorID is the calling admin.
func (s *Service) Update(ctx context.Context, actorID uuid.UUID, id string, in UpdateInput) (*userdom.WithRoles, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}

	if in.Email != nil {
		email, err := normalizeEmail(*in.Email)
		if err != nil {
			return nil, err
		}
		in.Email = &email
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || len(n) > 120 {
			return nil, apperr.Invalid("name", "name must be 1-120 characters")
		}
		in.Name = &n
	}
	if in.Status != nil {
		switch *in.Status {
		case userdom.StatusActive, userdom.StatusSuspended:
		default:
			return nil, apperr.Invalid("status", "status must be active or suspended")
		}
		if uid == actorID && *in.Status == userdom.StatusSuspended {
			return nil, apperr.Invalid("status", "you cannot suspend your own account")
		}
	}
	if in.Role != nil {
		if err := s.ensureRoleExists(ctx, *in.Role); err != nil {
			return nil, err
		}
	}

	// Guard: suspending or demoting the last active owner locks everyone out.
	if (in.Status != nil && *in.Status == userdom.StatusSuspended) ||
		(in.Role != nil && *in.Role != "owner") {
		if err := s.guardLastOwner(ctx, uid); err != nil {
			return nil, err
		}
	}

	if in.Name != nil || in.Email != nil {
		if err := s.repo.UpdateProfile(ctx, uid, in.Name, in.Email); err != nil {
			return nil, err
		}
	}
	if in.Status != nil {
		if err := s.repo.SetStatus(ctx, uid, *in.Status); err != nil {
			return nil, err
		}
	}
	if in.Role != nil {
		if err := s.repo.SetGlobalRole(ctx, uid, *in.Role); err != nil {
			return nil, err
		}
	}

	u, err := s.repo.GetByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	roles, err := s.repo.RolesFor(ctx, uid)
	if err != nil {
		return nil, err
	}
	return &userdom.WithRoles{User: u, Roles: roles}, nil
}

// ResetPassword sets a new password for a user (admin action).
func (s *Service) ResetPassword(ctx context.Context, id, newPassword string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return apperr.Internal(err)
	}
	return s.repo.SetPassword(ctx, uid, hash)
}

// Delete permanently removes a user; actorID is the calling admin.
func (s *Service) Delete(ctx context.Context, actorID uuid.UUID, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	if uid == actorID {
		return apperr.Invalid("id", "you cannot delete your own account")
	}
	if err := s.guardLastOwner(ctx, uid); err != nil {
		return err
	}
	return s.repo.Delete(ctx, uid)
}

// ---- Self-service profile ----

// Profile returns the caller's account with roles.
func (s *Service) Profile(ctx context.Context, userID uuid.UUID) (*userdom.WithRoles, error) {
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	roles, err := s.repo.RolesFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &userdom.WithRoles{User: u, Roles: roles}, nil
}

// UpdateProfile lets the caller change their own name/email.
func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, name, email *string) (*userdom.WithRoles, error) {
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" || len(n) > 120 {
			return nil, apperr.Invalid("name", "name must be 1-120 characters")
		}
		name = &n
	}
	if email != nil {
		e, err := normalizeEmail(*email)
		if err != nil {
			return nil, err
		}
		email = &e
	}
	if name == nil && email == nil {
		return nil, apperr.Invalid("body", "nothing to update")
	}
	if err := s.repo.UpdateProfile(ctx, userID, name, email); err != nil {
		return nil, err
	}
	return s.Profile(ctx, userID)
}

// ChangePassword verifies the current password before setting a new one.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, current, next string) error {
	if err := validatePassword(next); err != nil {
		return err
	}
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	creds, err := s.repo.GetCredentialsByEmail(ctx, u.Email)
	if err != nil {
		return apperr.Internal(err)
	}
	if !auth.CheckPassword(creds.Hash, current) {
		return apperr.Invalid("current_password", "current password is incorrect")
	}
	hash, err := auth.HashPassword(next)
	if err != nil {
		return apperr.Internal(err)
	}
	return s.repo.SetPassword(ctx, userID, hash)
}

// ---- helpers ----

// guardLastOwner fails if uid is the only remaining active owner.
func (s *Service) guardLastOwner(ctx context.Context, uid uuid.UUID) error {
	roles, err := s.repo.RolesFor(ctx, uid)
	if err != nil {
		return err
	}
	isOwner := false
	for _, r := range roles {
		if r == "owner" {
			isOwner = true
			break
		}
	}
	if !isOwner {
		return nil
	}
	n, err := s.repo.CountActiveOwners(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return apperr.Conflict("this is the last active owner account; assign another owner first")
	}
	return nil
}

func (s *Service) ensureRoleExists(ctx context.Context, role string) error {
	roles, err := s.repo.ListRoles(ctx)
	if err != nil {
		return err
	}
	for _, r := range roles {
		if r.Name == role {
			return nil
		}
	}
	return apperr.Invalid("role", "unknown role")
}

func normalizeEmail(raw string) (string, error) {
	email := strings.TrimSpace(strings.ToLower(raw))
	if email == "" {
		return "", apperr.Invalid("email", "email is required")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return "", apperr.Invalid("email", "email is not valid")
	}
	return email, nil
}

func validatePassword(pw string) error {
	if len(pw) < minPasswordLen {
		return apperr.Invalid("password", "password must be at least 8 characters")
	}
	if len(pw) > 128 {
		return apperr.Invalid("password", "password must be at most 128 characters")
	}
	return nil
}
