package dbadminapp

import (
	"context"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// withExtensions runs fn as the instance's admin login: installing an
// extension usually needs a superuser, and what is offered depends on the
// server, not on Fleetdock's data roles.
func (s *Service) withExtensions(ctx context.Context, databaseID string, write bool,
	fn func(ctx context.Context, x engine.Extensions, conn engine.ConnParams, db *databasedom.Database) error) error {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return err
	}
	if write && db.Status != databasedom.StatusActive {
		return apperr.Conflict("database is " + string(db.Status) + "; extensions cannot be changed now")
	}
	x, ok := admin.(engine.Extensions)
	if !ok {
		return apperr.Invalid("engine", "extensions are a PostgreSQL feature")
	}
	timeout := opTimeout
	if write {
		timeout = ddlTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := fn(cctx, x, conn, db); err != nil {
		return apperr.FromEngine(err, "extension")
	}
	if write {
		// An extension brings types, functions and sometimes tables.
		s.refreshGrants(ctx, db)
		if s.schemaChanged != nil {
			s.schemaChanged(db.ID)
		}
	}
	return nil
}

// Extensions lists the extensions the server offers and those installed.
func (s *Service) Extensions(ctx context.Context, databaseID string) ([]engine.Extension, error) {
	var out []engine.Extension
	err := s.withExtensions(ctx, databaseID, false, func(ctx context.Context, x engine.Extensions, conn engine.ConnParams, db *databasedom.Database) error {
		var err error
		out, err = x.Extensions(ctx, conn, db.Name)
		return err
	})
	return out, err
}

// InstallExtension installs an extension into a schema.
func (s *Service) InstallExtension(ctx context.Context, databaseID, name, schema, version string) error {
	return s.withExtensions(ctx, databaseID, true, func(ctx context.Context, x engine.Extensions, conn engine.ConnParams, db *databasedom.Database) error {
		return x.InstallExtension(ctx, conn, db.Name, name, schema, version)
	})
}

// UpdateExtension updates an extension to a version, or the latest.
func (s *Service) UpdateExtension(ctx context.Context, databaseID, name, version string) error {
	return s.withExtensions(ctx, databaseID, true, func(ctx context.Context, x engine.Extensions, conn engine.ConnParams, db *databasedom.Database) error {
		return x.UpdateExtension(ctx, conn, db.Name, name, version)
	})
}

// DropExtension drops an extension.
func (s *Service) DropExtension(ctx context.Context, databaseID, name string, cascade bool) error {
	return s.withExtensions(ctx, databaseID, true, func(ctx context.Context, x engine.Extensions, conn engine.ConnParams, db *databasedom.Database) error {
		return x.DropExtension(ctx, conn, db.Name, name, cascade)
	})
}
