// Package console models SQL console history and saved queries. Both are
// private to the user who created them.
package console

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// HistoryEntry is one console run.
type HistoryEntry struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"-"`
	DatabaseID uuid.UUID `json:"database_id"`
	SQL        string    `json:"sql"`
	Statements int       `json:"statements"`
	DurationMS int64     `json:"duration_ms"`
	RowCount   int64     `json:"row_count"`
	Error      *string   `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// SavedQuery is a named query, optionally tied to one database.
type SavedQuery struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"-"`
	DatabaseID *uuid.UUID `json:"database_id"`
	Name       string     `json:"name"`
	SQL        string     `json:"sql"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// HistoryLimit is how many runs are kept per user and database.
const HistoryLimit = 200

// Repository persists history and saved queries. Every method is scoped to a
// user: one user can never read or change another's entries.
type Repository interface {
	AddHistory(ctx context.Context, e *HistoryEntry) error
	ListHistory(ctx context.Context, userID, databaseID uuid.UUID, limit int) ([]HistoryEntry, error)
	ListSaved(ctx context.Context, userID uuid.UUID, databaseID *uuid.UUID) ([]SavedQuery, error)
	CreateSaved(ctx context.Context, q *SavedQuery) error
	UpdateSaved(ctx context.Context, userID, id uuid.UUID, name, sql string) (*SavedQuery, error)
	DeleteSaved(ctx context.Context, userID, id uuid.UUID) error
}
