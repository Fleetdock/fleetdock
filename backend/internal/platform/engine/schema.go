package engine

import (
	"context"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/schemadiff"
)

// SchemaReader reads a database's whole structure for schema history and
// drift detection.
type SchemaReader interface {
	// ReadSchema returns the database's structure, canonicalised. Schema.Engine
	// is left for the caller to set.
	ReadSchema(ctx context.Context, p ConnParams, database string) (*schemadiff.Schema, error)
}

// Schema read limits: beyond them the schema is marked truncated.
const (
	maxSchemaTables   = 2000
	maxSchemaRoutines = 500
)
