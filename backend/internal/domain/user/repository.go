package user

import (
	"context"

	"github.com/google/uuid"
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
	// PermissionsFor returns the distinct permission strings granted to a user
	// via its global role assignments.
	PermissionsFor(ctx context.Context, id uuid.UUID) ([]string, error)

	// CountUsers returns the number of user accounts (for bootstrap).
	CountUsers(ctx context.Context) (int, error)
	// CreateWithRole creates a user with a password hash and grants a global role.
	CreateWithRole(ctx context.Context, u *User, passwordHash, roleName string) error
}
