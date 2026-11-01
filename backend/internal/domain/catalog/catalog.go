// Package catalog is the fleet-wide index of table and column names that
// backs schema search: which databases, on which servers, have a table or a
// column with a given name.
package catalog

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// MaxSearchLimit caps how many hits one search returns.
const MaxSearchLimit = 50

// Hit is one search result: a table, or a column of a table.
type Hit struct {
	DatabaseID   uuid.UUID `json:"database_id"`
	DatabaseName string    `json:"database"`
	InstanceID   uuid.UUID `json:"instance_id"`
	InstanceName string    `json:"instance"`
	Engine       string    `json:"engine"`
	// Schema is the PostgreSQL schema ("" on MySQL/MariaDB).
	Schema string `json:"schema,omitempty"`
	Table  string `json:"table"`
	Kind   string `json:"kind"`
	// Column is set when the hit is a column.
	Column *string `json:"column,omitempty"`
	Type   string  `json:"type,omitempty"`
}

// Repository persists the catalog.
type Repository interface {
	// Replace swaps a database's catalog for tables, recording the refresh.
	Replace(ctx context.Context, databaseID uuid.UUID, cat *engine.ColumnCatalog, at time.Time) error
	// Failed records a refresh attempt that could not read the database.
	Failed(ctx context.Context, databaseID uuid.UUID, msg string, at time.Time) error
	// ListDue returns active, non-system databases on instances with
	// credentials whose catalog is older than staleBefore (or missing), and
	// whose last attempt is older than retryBefore, oldest first.
	ListDue(ctx context.Context, staleBefore, retryBefore time.Time, limit int) ([]uuid.UUID, error)
	// Search matches table and column names containing q (case-insensitive),
	// tables before columns and prefix matches first.
	Search(ctx context.Context, q string, limit int) ([]Hit, error)
}
