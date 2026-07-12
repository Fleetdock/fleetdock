package user

import (
	"context"

	"github.com/google/uuid"

	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
)

// Credentials bundles a user with its stored password hash for authentication.
type Credentials struct {
	User User
	Hash string
}

// Repository is the persistence port for users and their permissions.
type Repository interface {
	// GetCredentialsByEmail returns the user and password hash for login.
	GetCredentialsByEmail(ctx context.Context, email string) (Credentials, error)
	// GetByID returns a user by id.
	GetByID(ctx context.Context, id uuid.UUID) (User, error)
	// GrantsFor returns the flattened (permission, scope) grants a user holds
	// across all of its role assignments (global and scoped).
	GrantsFor(ctx context.Context, id uuid.UUID) ([]authz.Grant, error)

	// CountUsers returns the number of user accounts (for bootstrap).
	CountUsers(ctx context.Context) (int, error)
	// CreateWithRole creates a user with a password hash and grants a global role.
	CreateWithRole(ctx context.Context, u *User, passwordHash, roleName string) error
}
