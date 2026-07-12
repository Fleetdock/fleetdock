package user

import (
	"context"

	"github.com/google/uuid"
)

// Statuses for operator accounts.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusInvited   = "invited"
)

// WithRoles is a user plus its globally assigned role names.
type WithRoles struct {
	User
	Roles []string
}

// Role is a named permission set.
type Role struct {
	ID          uuid.UUID
	Name        string
	Description string
	IsSystem    bool
	Permissions []string
}

// RoleGrant is a role assignment to a user at a scope (global, server, or
// database). ScopeID is nil when ScopeType is "global".
type RoleGrant struct {
	ID        uuid.UUID
	RoleID    uuid.UUID
	RoleName  string
	ScopeType string
	ScopeID   *uuid.UUID
}

// PermissionCatalog is the full set of permissions the API enforces.
// requirePerm() strings in the HTTP router must stay within this set.
var PermissionCatalog = []string{
	"server:read", "server:write",
	"instance:read", "instance:write",
	"database:read", "database:write",
	"operation:read", "operation:write",
	"backup:read", "backup:write",
	"destination:read", "destination:write",
	"schedule:read", "schedule:write",
  "notification:read", "notification:write",
  "user:read", "user:write",
	"token:read", "token:write",
}

// AdminRepository extends the persistence port with account-management
// operations (implemented by the same Postgres adapter).
type AdminRepository interface {
	// List returns all users with their global role names.
	List(ctx context.Context) ([]WithRoles, error)
	// RolesFor returns the global role names assigned to a user.
	RolesFor(ctx context.Context, id uuid.UUID) ([]string, error)
	// UpdateProfile changes name and/or email (nil = keep).
	UpdateProfile(ctx context.Context, id uuid.UUID, name, email *string) error
	// SetPassword replaces the stored password hash.
	SetPassword(ctx context.Context, id uuid.UUID, hash string) error
	// SetStatus transitions the account status.
	SetStatus(ctx context.Context, id uuid.UUID, status string) error
	// SetGlobalRole replaces all global role grants with the named role.
	SetGlobalRole(ctx context.Context, id uuid.UUID, roleName string) error
	// ListGrants returns all role grants (global and scoped) for a user.
	ListGrants(ctx context.Context, userID uuid.UUID) ([]RoleGrant, error)
	// GetGrant returns one grant belonging to a user.
	GetGrant(ctx context.Context, userID, grantID uuid.UUID) (RoleGrant, error)
	// AddGrant assigns a role to a user at a scope (scopeID nil for global).
	AddGrant(ctx context.Context, userID uuid.UUID, roleName, scopeType string, scopeID *uuid.UUID) (RoleGrant, error)
	// RemoveGrant deletes a user's grant by id.
	RemoveGrant(ctx context.Context, userID, grantID uuid.UUID) error
	// Delete permanently removes the account.
	Delete(ctx context.Context, id uuid.UUID) error
	// CountActiveOwners counts active users holding the owner role globally
	// (used to protect against locking everyone out).
	CountActiveOwners(ctx context.Context) (int, error)
	// ListRoles returns the role catalog with permissions.
	ListRoles(ctx context.Context) ([]Role, error)
	// GetRole returns one role with its permissions.
	GetRole(ctx context.Context, id uuid.UUID) (*Role, error)
	// CreateRole inserts a custom (non-system) role with permissions.
	CreateRole(ctx context.Context, r *Role) error
	// UpdateRole replaces name/description/permissions of a role.
	UpdateRole(ctx context.Context, id uuid.UUID, name, description *string, permissions []string) error
	// DeleteRole removes a role.
	DeleteRole(ctx context.Context, id uuid.UUID) error
	// CountRoleAssignments counts user_roles rows referencing the role.
	CountRoleAssignments(ctx context.Context, id uuid.UUID) (int, error)
}
