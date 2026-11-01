package dbadminapp

import (
	"context"

	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	dbaccessdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbaccess"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/schemadiff"
)

func catalog(admin engine.Admin) (engine.Catalog, error) {
	c, ok := admin.(engine.Catalog)
	if !ok {
		return nil, apperr.Invalid("engine", "this engine does not support catalog introspection")
	}
	return c, nil
}

// Columns lists the database's tables and views with their columns, as the
// data login sees them — what the SQL editor completes.
func (s *Service) Columns(ctx context.Context, databaseID string) (*engine.ColumnCatalog, error) {
	var out *engine.ColumnCatalog
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			cat, err := catalog(admin)
			if err != nil {
				return err
			}
			out, err = cat.Columns(ctx, conn, db.Name)
			return err
		})
	return out, err
}

// ReferencedBy lists the foreign keys that reference a table.
func (s *Service) ReferencedBy(ctx context.Context, databaseID, table string) ([]engine.ForeignKey, error) {
	var out []engine.ForeignKey
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			cat, err := catalog(admin)
			if err != nil {
				return err
			}
			out, err = cat.ReferencedBy(ctx, conn, db.Name, table)
			return err
		})
	return out, err
}

// SetSchemaChanged registers a callback for DDL made through Fleetdock (the
// structure editor). It must not block: it runs on the request path.
func (s *Service) SetSchemaChanged(fn func(databaseID uuid.UUID)) { s.schemaChanged = fn }

// AdminColumns reads a database's column catalog with the instance's admin
// login. It feeds the background fleet-wide catalog, which indexes every
// table once; search results are filtered per user afterwards.
func (s *Service) AdminColumns(ctx context.Context, databaseID uuid.UUID) (*engine.ColumnCatalog, error) {
	db, err := s.databases.GetByID(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	_, admin, conn, err := s.target(ctx, db.InstanceID.String())
	if err != nil {
		return nil, err
	}
	cat, err := catalog(admin)
	if err != nil {
		return nil, err
	}
	conn.Database = db.Name
	return cat.Columns(ctx, conn, db.Name)
}

func schemaReader(admin engine.Admin) (engine.SchemaReader, error) {
	r, ok := admin.(engine.SchemaReader)
	if !ok {
		return nil, apperr.Invalid("engine", "this engine does not support reading the schema")
	}
	return r, nil
}

// Schema reads the database's whole structure as the data login sees it.
func (s *Service) Schema(ctx context.Context, databaseID string) (*schemadiff.Schema, error) {
	var out *schemadiff.Schema
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			r, err := schemaReader(admin)
			if err != nil {
				return err
			}
			out, err = r.ReadSchema(ctx, conn, db.Name)
			return err
		})
	return out, err
}

// AdminSchema reads the database's structure with the instance's admin
// login, for the background schema history.
func (s *Service) AdminSchema(ctx context.Context, databaseID uuid.UUID) (*schemadiff.Schema, error) {
	db, err := s.databases.GetByID(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	_, admin, conn, err := s.target(ctx, db.InstanceID.String())
	if err != nil {
		return nil, err
	}
	r, err := schemaReader(admin)
	if err != nil {
		return nil, err
	}
	conn.Database = db.Name
	return r.ReadSchema(ctx, conn, db.Name)
}
