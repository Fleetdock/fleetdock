package engine

import (
	"context"
	"fmt"
)

// Catalog is bulk introspection of a whole database: one round trip answers a
// question about every table, where the Admin and Structure surfaces ask per
// table. It backs editor autocompletion, the fleet-wide schema search and
// "referenced by" navigation.
type Catalog interface {
	// Columns lists the tables and views of a database with their columns,
	// in ordinal order. Only objects the login can read are included.
	Columns(ctx context.Context, p ConnParams, database string) (*ColumnCatalog, error)
	// ReferencedBy lists the foreign keys of other tables (and the table
	// itself) that point at table: its incoming foreign keys.
	ReferencedBy(ctx context.Context, p ConnParams, database, table string) ([]ForeignKey, error)
}

// Catalog size limits: a database beyond them is reported truncated rather
// than shipped whole to the browser.
const (
	maxCatalogTables  = 5000
	maxCatalogColumns = 100000
)

// CatalogColumn is one column of a catalog table.
type CatalogColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CatalogTable is a table or view with its columns. Schema is the
// PostgreSQL schema; on MySQL/MariaDB it is empty (the database is the schema).
type CatalogTable struct {
	Schema  string          `json:"schema,omitempty"`
	Name    string          `json:"name"`
	Kind    string          `json:"kind"` // "table" or "view"
	Columns []CatalogColumn `json:"columns"`
}

// ColumnCatalog is the column listing of one database.
type ColumnCatalog struct {
	Tables []CatalogTable `json:"tables"`
	// Truncated reports that the database has more tables or columns than
	// the catalog limits and only the first ones are listed.
	Truncated bool `json:"truncated"`
}

// catalogBuilder groups ordered (schema, table, column) rows into tables and
// enforces the size limits.
type catalogBuilder struct {
	out     ColumnCatalog
	columns int
}

// add appends one column row; it returns false once the limits are reached.
func (b *catalogBuilder) add(schema, table, kind, column, typ string) bool {
	n := len(b.out.Tables)
	if n == 0 || b.out.Tables[n-1].Schema != schema || b.out.Tables[n-1].Name != table {
		if n >= maxCatalogTables {
			b.out.Truncated = true
			return false
		}
		b.out.Tables = append(b.out.Tables, CatalogTable{Schema: schema, Name: table, Kind: kind})
		n++
	}
	if b.columns >= maxCatalogColumns {
		b.out.Truncated = true
		return false
	}
	b.columns++
	t := &b.out.Tables[n-1]
	t.Columns = append(t.Columns, CatalogColumn{Name: column, Type: typ})
	return true
}

func (b *catalogBuilder) result() *ColumnCatalog {
	if b.out.Tables == nil {
		b.out.Tables = []CatalogTable{}
	}
	return &b.out
}

// CatalogFor returns the bulk-introspection surface for the engine name.
func CatalogFor(name string) (Catalog, error) {
	c, err := For(name)
	if err != nil {
		return nil, err
	}
	cat, ok := c.(Catalog)
	if !ok {
		return nil, fmt.Errorf("engine: %s does not support catalog introspection", name)
	}
	return cat, nil
}
