package dbadminapp

import (
	"context"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

func schemaPrivileges(admin engine.Admin) (engine.SchemaPrivileges, error) {
	sp, ok := admin.(engine.SchemaPrivileges)
	if !ok {
		return nil, apperr.Invalid("engine", "schema privileges and search_path are PostgreSQL settings")
	}
	return sp, nil
}

// withPrivileges runs fn against a database as the instance's admin login:
// only a schema's owner, a superuser or the role itself may change who may
// use a schema, its default privileges or a search_path. write refuses a
// database that is not active.
func (s *Service) withPrivileges(ctx context.Context, databaseID string, write bool,
	fn func(ctx context.Context, sp engine.SchemaPrivileges, conn engine.ConnParams, db *databasedom.Database) error) error {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return err
	}
	if write && db.Status != databasedom.StatusActive {
		return apperr.Conflict("database is " + string(db.Status) + "; its privileges cannot be changed now")
	}
	sp, err := schemaPrivileges(admin)
	if err != nil {
		return err
	}
	timeout := opTimeout
	if write {
		timeout = ddlTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return apperr.FromEngine(fn(cctx, sp, conn, db), "schema")
}

// SchemaAccess reports who may use a schema and its default privileges.
func (s *Service) SchemaAccess(ctx context.Context, databaseID, schema string) (*engine.SchemaAccess, error) {
	var out *engine.SchemaAccess
	err := s.withPrivileges(ctx, databaseID, false, func(ctx context.Context, sp engine.SchemaPrivileges, conn engine.ConnParams, db *databasedom.Database) error {
		var err error
		out, err = sp.SchemaAccess(ctx, conn, db.Name, schema)
		return err
	})
	return out, err
}

// SetSchemaGrant leaves a role with exactly the USAGE and CREATE asked for.
func (s *Service) SetSchemaGrant(ctx context.Context, databaseID, schema, role string, usage, create bool) error {
	return s.withPrivileges(ctx, databaseID, true, func(ctx context.Context, sp engine.SchemaPrivileges, conn engine.ConnParams, db *databasedom.Database) error {
		return sp.SetSchemaGrant(ctx, conn, db.Name, schema, role, usage, create)
	})
}

// AlterDefaultPrivileges changes what objects created in a schema are
// granted.
func (s *Service) AlterDefaultPrivileges(ctx context.Context, databaseID, schema string, change engine.DefaultPrivilegeChange) error {
	return s.withPrivileges(ctx, databaseID, true, func(ctx context.Context, sp engine.SchemaPrivileges, conn engine.ConnParams, db *databasedom.Database) error {
		return sp.AlterDefaultPrivileges(ctx, conn, db.Name, schema, change)
	})
}

// SearchPaths lists the search_path of a database and of roles in it.
func (s *Service) SearchPaths(ctx context.Context, databaseID string) (*engine.SearchPaths, error) {
	var out *engine.SearchPaths
	err := s.withPrivileges(ctx, databaseID, false, func(ctx context.Context, sp engine.SchemaPrivileges, conn engine.ConnParams, db *databasedom.Database) error {
		var err error
		out, err = sp.SearchPaths(ctx, conn, db.Name)
		return err
	})
	return out, err
}

// SetSearchPath sets or resets the search_path of a database or of a role in
// it. New sessions pick it up; open ones keep theirs.
func (s *Service) SetSearchPath(ctx context.Context, databaseID, role string, schemas []string) error {
	return s.withPrivileges(ctx, databaseID, true, func(ctx context.Context, sp engine.SchemaPrivileges, conn engine.ConnParams, db *databasedom.Database) error {
		return sp.SetSearchPath(ctx, conn, db.Name, role, schemas)
	})
}
