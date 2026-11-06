package engine

import (
	"context"
	"fmt"
	"strings"
)

// Columns lists a database's tables and views with their columns. ClickHouse
// has no schemas within a database, so schemas is ignored.
func (c *ClickHouse) Columns(ctx context.Context, p ConnParams, database string, _ ...string) (*ColumnCatalog, error) {
	if !validDBName(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	rows, err := c.rows(ctx, p, chRequest{
		query: fmt.Sprintf(`
			SELECT c.table, if(t.engine IN ({views:Array(String)}), 'view', 'table'),
			       c.name, c.type, c.is_in_primary_key
			FROM system.columns c
			INNER JOIN system.tables t ON t.database = c.database AND t.name = c.table
			WHERE c.database = {db:String}
			ORDER BY c.table, c.position
			LIMIT %d`, maxCatalogColumns+1),
		params: map[string]string{"db": database, "views": chViewEnginesParam},
	})
	if err != nil {
		return nil, err
	}
	var b catalogBuilder
	for _, r := range rows {
		t := str(r[3])
		col := CatalogColumn{
			Name:       str(r[2]),
			Type:       t,
			Nullable:   strings.HasPrefix(t, "Nullable(") || strings.HasPrefix(t, "LowCardinality(Nullable("),
			PrimaryKey: str(r[4]) == "1",
		}
		if !b.add("", str(r[0]), str(r[1]), col) {
			break
		}
	}
	return b.result(), nil
}

// ReferencedBy: ClickHouse has no foreign keys.
func (c *ClickHouse) ReferencedBy(context.Context, ConnParams, string, string) ([]ForeignKey, error) {
	return []ForeignKey{}, nil
}

// Relations: ClickHouse has no foreign keys.
func (c *ClickHouse) Relations(context.Context, ConnParams, string, ...string) ([]ForeignKey, bool, error) {
	return []ForeignKey{}, false, nil
}

// SchemaCounts: ClickHouse databases have no schemas.
func (c *ClickHouse) SchemaCounts(context.Context, ConnParams, string) ([]SchemaCount, error) {
	return nil, nil
}

// chViewEngines are the table engines of views. Views are listed as objects
// (Objects), not tables, as on the other engines; materialized views are
// still queryable and browsable by name.
var chViewEngines = map[string]ObjectKind{
	"View":             ObjectView,
	"LiveView":         ObjectView,
	"WindowView":       ObjectView,
	"MaterializedView": ObjectMaterializedView,
}

// chViewEnginesParam is chViewEngines as an Array(String) query parameter.
const chViewEnginesParam = "['View','LiveView','WindowView','MaterializedView']"

// Objects lists the database's views and materialized views with their
// CREATE statements. ClickHouse has no routines, triggers, sequences or
// events scoped to a database.
func (c *ClickHouse) Objects(ctx context.Context, p ConnParams, database string) ([]DBObject, error) {
	if !validDBName(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	rows, err := c.rows(ctx, p, chRequest{
		query: `
			SELECT name, engine, create_table_query FROM system.tables
			WHERE database = {db:String} AND engine IN ({views:Array(String)})
			ORDER BY name`,
		params: map[string]string{"db": database, "views": chViewEnginesParam},
	})
	if err != nil {
		return nil, err
	}
	out := make([]DBObject, 0, len(rows))
	for _, r := range rows {
		out = append(out, DBObject{
			Kind: chViewEngines[str(r[1])], Schema: database, Name: str(r[0]),
			Definition: truncateDefinition(r[2]),
		})
	}
	return out, nil
}

// ForeignKeys: ClickHouse has no foreign keys.
func (c *ClickHouse) ForeignKeys(context.Context, ConnParams, string, string) ([]ForeignKey, error) {
	return []ForeignKey{}, nil
}
