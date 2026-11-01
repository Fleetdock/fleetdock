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

// DiagramTable is one table or view of an entity-relationship diagram.
type DiagramTable struct {
	// Schema is the PostgreSQL schema; empty on MySQL/MariaDB.
	Schema string `json:"schema,omitempty"`
	Name   string `json:"name"`
	Kind   string `json:"kind"` // "table" or "view"
	// RowEstimate is the engine's row estimate (statistics, not a count);
	// absent for views and tables the engine has no estimate for.
	RowEstimate *int64                 `json:"row_estimate,omitempty"`
	Columns     []engine.CatalogColumn `json:"columns"`
}

// Diagram is a database's tables and the foreign keys between them, as the
// data login sees them. Relation tables are named like DiagramTable
// identifiers: "schema.table" on PostgreSQL, "table" elsewhere.
type Diagram struct {
	Tables    []DiagramTable      `json:"tables"`
	Relations []engine.ForeignKey `json:"relations"`
	// Truncated reports that the database has more tables, columns or
	// foreign keys than are listed.
	Truncated bool `json:"truncated"`
}

// Diagram reads what an entity-relationship diagram of the database needs:
// tables with their columns and keys, row estimates and foreign keys.
func (s *Service) Diagram(ctx context.Context, databaseID string) (*Diagram, error) {
	var out *Diagram
	err := s.scoped(ctx, databaseID, dbaccessdom.ModeRead, opTimeout,
		func(ctx context.Context, db *databasedom.Database, admin engine.Admin, conn engine.ConnParams) error {
			cat, err := catalog(admin)
			if err != nil {
				return err
			}
			cols, err := cat.Columns(ctx, conn, db.Name)
			if err != nil {
				return err
			}
			fks, fkTruncated, err := cat.Relations(ctx, conn, db.Name)
			if err != nil {
				return err
			}
			// Row estimates are a nicety: a login that cannot read the
			// statistics still gets its diagram.
			tables, _ := admin.ListTables(ctx, conn, db.Name)
			out = buildDiagram(cols, fks, tables)
			out.Truncated = out.Truncated || fkTruncated
			return nil
		})
	return out, err
}

// buildDiagram joins the column catalog with row estimates.
func buildDiagram(cols *engine.ColumnCatalog, fks []engine.ForeignKey, tables []engine.TableInfo) *Diagram {
	rows := make(map[string]int64, len(tables))
	for _, t := range tables {
		rows[t.Schema+"\x00"+t.Name] = t.RowCount
		rows["\x00"+t.Name] = t.RowCount // MySQL/MariaDB report the database as Schema
	}
	out := &Diagram{Tables: make([]DiagramTable, 0, len(cols.Tables)), Relations: fks, Truncated: cols.Truncated}
	if out.Relations == nil {
		out.Relations = []engine.ForeignKey{}
	}
	for _, t := range cols.Tables {
		dt := DiagramTable{Schema: t.Schema, Name: t.Name, Kind: t.Kind, Columns: t.Columns}
		if dt.Columns == nil {
			dt.Columns = []engine.CatalogColumn{}
		}
		if t.Kind == "table" {
			if n, ok := rows[t.Schema+"\x00"+t.Name]; ok && n >= 0 {
				dt.RowEstimate = &n
			}
		}
		out.Tables = append(out.Tables, dt)
	}
	return out
}
