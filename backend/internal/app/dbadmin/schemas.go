package dbadminapp

import (
	"context"
	"log/slog"

	dbaccessapp "github.com/Fleetdock/fleetdock/backend/internal/app/dbaccess"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	dbaccessdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbaccess"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

func schemaAdmin(admin engine.Admin) (engine.SchemaAdmin, error) {
	sa, ok := admin.(engine.SchemaAdmin)
	if !ok {
		return nil, apperr.Invalid("engine", "this engine has no schemas: on MySQL and MariaDB a schema is a database")
	}
	return sa, nil
}

// withSchemaAdmin runs a schema change as the instance's admin login: schemas
// are often owned by roles Fleetdock's data roles are not members of, and
// renaming, re-owning or dropping one needs its owner or a superuser.
func (s *Service) withSchemaAdmin(ctx context.Context, databaseID string,
	fn func(ctx context.Context, sa engine.SchemaAdmin, conn engine.ConnParams, db *databasedom.Database) error) error {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return err
	}
	if db.Status != databasedom.StatusActive {
		return apperr.Conflict("database is " + string(db.Status) + "; schemas cannot be changed now")
	}
	sa, err := schemaAdmin(admin)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, ddlTimeout)
	defer cancel()
	if err := fn(cctx, sa, conn, db); err != nil {
		return apperr.FromEngine(err, "schema")
	}
	s.refreshGrants(ctx, db)
	if s.schemaChanged != nil {
		s.schemaChanged(db.ID)
	}
	return nil
}

// refreshGrants re-applies both data roles' grants after a schema change: on
// PostgreSQL they are granted per schema, so a new or renamed schema is out of
// their reach until they are. Best effort, like refreshReadGrants.
func (s *Service) refreshGrants(ctx context.Context, db *databasedom.Database) {
	inst, admin, root, err := s.target(ctx, db.InstanceID.String())
	if err != nil || inst.Engine != instancedom.EnginePostgres || inst.DataAccessOrDefault() != instancedom.DataAccessManaged {
		return
	}
	t := dbaccessapp.Target{Instance: inst, Database: db, Admin: admin, Root: root}
	for _, mode := range []dbaccessdom.Mode{dbaccessdom.ModeRead, dbaccessdom.ModeWrite} {
		if err := s.access.Reapply(ctx, t, mode); err != nil {
			slog.Warn("refresh data role grants after a schema change", "database_id", db.ID, "mode", mode, "error", err.Error())
		}
	}
}

// CreateSchema creates a PostgreSQL schema, owned by owner or by the
// database's owner.
func (s *Service) CreateSchema(ctx context.Context, databaseID, name, owner string) error {
	return s.withSchemaAdmin(ctx, databaseID, func(ctx context.Context, sa engine.SchemaAdmin, conn engine.ConnParams, db *databasedom.Database) error {
		return sa.CreateSchema(ctx, conn, db.Name, name, owner)
	})
}

// AlterSchema renames a schema, or changes its owner or comment.
func (s *Service) AlterSchema(ctx context.Context, databaseID, name string, change engine.SchemaChange) error {
	return s.withSchemaAdmin(ctx, databaseID, func(ctx context.Context, sa engine.SchemaAdmin, conn engine.ConnParams, db *databasedom.Database) error {
		return sa.AlterSchema(ctx, conn, db.Name, name, change)
	})
}

// SchemaDependents reports what dropping a schema would take with it.
func (s *Service) SchemaDependents(ctx context.Context, databaseID, name string) (*engine.SchemaDependents, error) {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	sa, err := schemaAdmin(admin)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	d, err := sa.SchemaDependents(cctx, conn, db.Name, name)
	return d, apperr.FromEngine(err, "schema")
}

// DropSchema drops a schema; without cascade only when it holds nothing.
func (s *Service) DropSchema(ctx context.Context, databaseID, name string, cascade bool) error {
	return s.withSchemaAdmin(ctx, databaseID, func(ctx context.Context, sa engine.SchemaAdmin, conn engine.ConnParams, db *databasedom.Database) error {
		return sa.DropSchema(ctx, conn, db.Name, name, cascade)
	})
}
