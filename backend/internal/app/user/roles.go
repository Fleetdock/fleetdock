package userapp

import (
	"context"
	"strings"
	"unicode"

	"github.com/google/uuid"

	userdom "github.com/TajBrains/db-manager/backend/internal/domain/user"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// PermissionCatalog returns every permission the API enforces.
func (s *Service) PermissionCatalog() []string {
	return userdom.PermissionCatalog
}

// RoleInput describes a custom role create/update.
type RoleInput struct {
	Name        *string
	Description *string
	Permissions []string // nil = keep existing (update only)
}

// CreateRole validates and creates a custom role.
func (s *Service) CreateRole(ctx context.Context, in RoleInput) (*userdom.Role, error) {
	if in.Name == nil {
		return nil, apperr.Invalid("name", "name is required")
	}
	name, err := validateRoleName(*in.Name)
	if err != nil {
		return nil, err
	}
	if in.Permissions == nil {
		in.Permissions = []string{}
	}
	perms, err := validatePermissions(in.Permissions)
	if err != nil {
		return nil, err
	}
	desc := ""
	if in.Description != nil {
		desc = strings.TrimSpace(*in.Description)
	}

	ro := &userdom.Role{
		ID:          uuid.New(),
		Name:        name,
		Description: desc,
		IsSystem:    false,
		Permissions: perms,
	}
	if err := s.repo.CreateRole(ctx, ro); err != nil {
		return nil, err
	}
	return ro, nil
}

// UpdateRole edits a custom role; system roles are immutable.
func (s *Service) UpdateRole(ctx context.Context, id string, in RoleInput) (*userdom.Role, error) {
	rid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	existing, err := s.repo.GetRole(ctx, rid)
	if err != nil {
		return nil, err
	}
	if existing.IsSystem {
		return nil, apperr.Invalid("id", "system roles cannot be modified")
	}

	if in.Name != nil {
		name, err := validateRoleName(*in.Name)
		if err != nil {
			return nil, err
		}
		in.Name = &name
	}
	if in.Description != nil {
		d := strings.TrimSpace(*in.Description)
		in.Description = &d
	}
	var perms []string
	if in.Permissions != nil {
		perms, err = validatePermissions(in.Permissions)
		if err != nil {
			return nil, err
		}
	}

	if err := s.repo.UpdateRole(ctx, rid, in.Name, in.Description, perms); err != nil {
		return nil, err
	}
	return s.repo.GetRole(ctx, rid)
}

// DeleteRole removes a custom role that is not assigned to anyone.
func (s *Service) DeleteRole(ctx context.Context, id string) error {
	rid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	existing, err := s.repo.GetRole(ctx, rid)
	if err != nil {
		return err
	}
	if existing.IsSystem {
		return apperr.Invalid("id", "system roles cannot be deleted")
	}
	n, err := s.repo.CountRoleAssignments(ctx, rid)
	if err != nil {
		return err
	}
	if n > 0 {
		return apperr.Conflict("role is assigned to users; reassign them first")
	}
	return s.repo.DeleteRole(ctx, rid)
}

func validateRoleName(raw string) (string, error) {
	name := strings.TrimSpace(strings.ToLower(raw))
	if len(name) < 2 || len(name) > 40 {
		return "", apperr.Invalid("name", "name must be 2-40 characters")
	}
	for _, r := range name {
		if !unicode.IsLower(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return "", apperr.Invalid("name", "name may only contain lowercase letters, digits, '-' and '_'")
		}
	}
	return name, nil
}

func validatePermissions(perms []string) ([]string, error) {
	valid := make(map[string]struct{}, len(userdom.PermissionCatalog))
	for _, p := range userdom.PermissionCatalog {
		valid[p] = struct{}{}
	}
	seen := make(map[string]struct{}, len(perms))
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		if _, ok := valid[p]; !ok {
			return nil, apperr.Invalid("permissions", "unknown permission: "+p)
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out, nil
}
