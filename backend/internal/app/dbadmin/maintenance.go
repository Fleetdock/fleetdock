package dbadminapp

import (
	"context"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// withMaintenance runs fn against a database as the instance's admin login:
// VACUUM, REINDEX, REFRESH and setval need the object's owner (or a
// superuser), and statistics are only complete for a login that sees every
// table. write refuses a database that is not active and allows the longer
// DDL timeout.
func (s *Service) withMaintenance(ctx context.Context, databaseID string, write bool,
	fn func(ctx context.Context, m engine.Maintenance, conn engine.ConnParams, db *databasedom.Database) error) error {
	db, admin, conn, err := s.databaseTarget(ctx, databaseID)
	if err != nil {
		return err
	}
	if write && db.Status != databasedom.StatusActive {
		return apperr.Conflict("database is " + string(db.Status) + "; maintenance cannot run now")
	}
	m, ok := admin.(engine.Maintenance)
	if !ok {
		return apperr.Invalid("engine", "this maintenance is PostgreSQL's")
	}
	timeout := opTimeout
	if write {
		timeout = ddlTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return apperr.FromEngine(fn(cctx, m, conn, db), "object")
}

// Sequence reads a sequence and its current value.
func (s *Service) Sequence(ctx context.Context, databaseID, schema, name string) (*engine.SequenceInfo, error) {
	var out *engine.SequenceInfo
	err := s.withMaintenance(ctx, databaseID, false, func(ctx context.Context, m engine.Maintenance, conn engine.ConnParams, db *databasedom.Database) error {
		var err error
		out, err = m.Sequence(ctx, conn, db.Name, schema, name)
		return err
	})
	return out, err
}

// SetSequence sets a sequence's value.
func (s *Service) SetSequence(ctx context.Context, databaseID, schema, name string, value int64, called bool) error {
	return s.withMaintenance(ctx, databaseID, true, func(ctx context.Context, m engine.Maintenance, conn engine.ConnParams, db *databasedom.Database) error {
		return m.SetSequence(ctx, conn, db.Name, schema, name, value, called)
	})
}

// RefreshMaterializedView recomputes a materialized view.
func (s *Service) RefreshMaterializedView(ctx context.Context, databaseID, schema, name string, concurrently bool) error {
	return s.withMaintenance(ctx, databaseID, true, func(ctx context.Context, m engine.Maintenance, conn engine.ConnParams, db *databasedom.Database) error {
		return m.RefreshMaterializedView(ctx, conn, db.Name, schema, name, concurrently)
	})
}

// Maintain runs VACUUM, ANALYZE or REINDEX on a table or a schema's tables.
func (s *Service) Maintain(ctx context.Context, databaseID string, op engine.MaintenanceOp, schema, table string) ([]string, error) {
	if !op.Valid() {
		return nil, apperr.Invalid("op", "op must be vacuum, vacuum_analyze, analyze or reindex")
	}
	if schema == "" {
		return nil, apperr.Invalid("schema", "schema is required")
	}
	var ran []string
	err := s.withMaintenance(ctx, databaseID, true, func(ctx context.Context, m engine.Maintenance, conn engine.ConnParams, db *databasedom.Database) error {
		var err error
		ran, err = m.Maintain(ctx, conn, db.Name, op, schema, table)
		// Fresh statistics change the row estimates the catalog shows.
		if err == nil && op != engine.OpReindex && s.schemaChanged != nil {
			s.schemaChanged(db.ID)
		}
		return err
	})
	return ran, err
}

// TableStats reads tables' sizes, dead rows and last vacuum and analyze.
func (s *Service) TableStats(ctx context.Context, databaseID, schema string) ([]engine.TableStat, error) {
	var out []engine.TableStat
	err := s.withMaintenance(ctx, databaseID, false, func(ctx context.Context, m engine.Maintenance, conn engine.ConnParams, db *databasedom.Database) error {
		var err error
		out, err = m.TableStats(ctx, conn, db.Name, schema)
		return err
	})
	return out, err
}

// Partitions lists a partitioned table's partitions.
func (s *Service) Partitions(ctx context.Context, databaseID, schema, table string) ([]engine.Partition, error) {
	var out []engine.Partition
	err := s.withMaintenance(ctx, databaseID, false, func(ctx context.Context, m engine.Maintenance, conn engine.ConnParams, db *databasedom.Database) error {
		var err error
		out, err = m.Partitions(ctx, conn, db.Name, schema, table)
		return err
	})
	return out, err
}
