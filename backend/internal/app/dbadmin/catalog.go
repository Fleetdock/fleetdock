package dbadminapp

import (
	"context"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	dbaccessdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbaccess"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
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
