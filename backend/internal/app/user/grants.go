package userapp

import (
	"context"

	"github.com/google/uuid"

	userdom "github.com/Fleetdock/fleetdock/backend/internal/domain/user"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// Scope types for role grants.
const (
	scopeGlobal   = "global"
	scopeServer   = "server"
	scopeDatabase = "database"
)

// ListGrants returns a user's role grants (global and scoped).
func (s *Service) ListGrants(ctx context.Context, id string) ([]userdom.RoleGrant, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.repo.ListGrants(ctx, uid)
}

// AddGrantInput describes a scoped role assignment.
type AddGrantInput struct {
	Role      string
	ScopeType string
	ScopeID   string // empty for global scope
}

// AddGrant assigns a role to a user at a scope.
func (s *Service) AddGrant(ctx context.Context, id string, in AddGrantInput) (userdom.RoleGrant, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return userdom.RoleGrant{}, apperr.Invalid("id", "id must be a valid UUID")
	}
	switch in.ScopeType {
	case scopeGlobal, scopeServer, scopeDatabase:
	default:
		return userdom.RoleGrant{}, apperr.Invalid("scope_type", "scope_type must be global, server or database")
	}
	if err := s.ensureRoleExists(ctx, in.Role); err != nil {
		return userdom.RoleGrant{}, err
	}

	var scopeID *uuid.UUID
	if in.ScopeType == scopeGlobal {
		if in.ScopeID != "" {
			return userdom.RoleGrant{}, apperr.Invalid("scope_id", "scope_id must be empty for global scope")
		}
	} else {
		sid, err := uuid.Parse(in.ScopeID)
		if err != nil {
			return userdom.RoleGrant{}, apperr.Invalid("scope_id", "scope_id must be a valid UUID")
		}
		scopeID = &sid
	}
	return s.repo.AddGrant(ctx, uid, in.Role, in.ScopeType, scopeID)
}

// RemoveGrant revokes a user's role grant by id, guarding against removing the
// last global owner.
func (s *Service) RemoveGrant(ctx context.Context, id, grantID string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	gid, err := uuid.Parse(grantID)
	if err != nil {
		return apperr.Invalid("grant_id", "grant_id must be a valid UUID")
	}
	g, err := s.repo.GetGrant(ctx, uid, gid)
	if err != nil {
		return err
	}
	if g.RoleName == "owner" && g.ScopeType == scopeGlobal {
		if err := s.guardLastOwner(ctx, uid); err != nil {
			return err
		}
	}
	return s.repo.RemoveGrant(ctx, uid, gid)
}
