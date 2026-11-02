// Package tableview is the domain model of saved data-browser table views.
package tableview

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// View is a named view of one table: filters, search, sort and columns as
// the dashboard stores them (State is opaque here).
type View struct {
	ID         uuid.UUID       `json:"id"`
	DatabaseID uuid.UUID       `json:"database_id"`
	Table      string          `json:"table"`
	Name       string          `json:"name"`
	State      json.RawMessage `json:"state"`
	// Shared views are listed to everyone who can read the database.
	Shared    bool       `json:"shared"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
	// AuthorEmail is the creator's email, for "shared by" in the list.
	AuthorEmail string    `json:"author_email,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Repository persists views.
type Repository interface {
	// List returns the user's own views of a database (of one table when
	// table is set) and everyone's shared ones, by name.
	List(ctx context.Context, databaseID uuid.UUID, table string, userID uuid.UUID) ([]View, error)
	Get(ctx context.Context, databaseID, id uuid.UUID) (*View, error)
	Create(ctx context.Context, v *View) error
	Update(ctx context.Context, v *View) error
	Delete(ctx context.Context, databaseID, id uuid.UUID) error
}
