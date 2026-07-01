// Package user is the domain model for platform operators (not managed DB users).
package user

import (
	"time"

	"github.com/google/uuid"
)

// User is an operator account that can sign in and act on the platform.
type User struct {
	ID        uuid.UUID
	Email     string
	Name      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}
