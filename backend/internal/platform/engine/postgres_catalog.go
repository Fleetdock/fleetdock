package engine

import (
	"context"
	"fmt"
)

var _ Catalog = (*Postgres)(nil)

// pgUserSchema restricts a namespace column to non-system schemas.
func pgUserSchema(col string) string {
	return col + ` NOT IN ('pg_catalog','information_schema') AND ` + col + ` NOT LIKE 'pg\_toast%' AND ` +
		col + ` NOT LIKE 'pg\_temp%'`
}

// Columns lists the tables, views, materialized views and foreign tables of
// every user schema with their columns. Partitions are left out (their parent
// is listed), as are relations the login has no privilege on — pg_catalog
// lists them all, unlike information_schema.
func (pg *Postgres) Columns(ctx context.Context, p ConnParams, database string) (*ColumnCatalog, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `
		SELECT n.nspname, c.relname, c.relkind::text, a.attname, format_type(a.atttypid, a.atttypmod)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		WHERE c.relkind IN ('r','p','v','m','f') AND NOT c.relispartition
		  AND `+pgUserSchema("n.nspname")+`
		  AND has_any_column_privilege(c.oid, 'SELECT')
		ORDER BY n.nspname, c.relname, a.attnum`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var b catalogBuilder
	for rows.Next() {
		var schema, table, relkind, col, typ string
		if err := rows.Scan(&schema, &table, &relkind, &col, &typ); err != nil {
			return nil, err
		}
		kind := "table"
		if relkind == "v" || relkind == "m" {
			kind = "view"
		}
		if !b.add(schema, table, kind, col, typ) {
			break
		}
	}
	return b.result(), rows.Err()
}

// ReferencedBy lists the foreign keys that reference table. Referencing
// tables are named "schema.table", as everywhere on PostgreSQL.
func (pg *Postgres) ReferencedBy(ctx context.Context, p ConnParams, database, table string) ([]ForeignKey, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	if !validTableName(table) {
		return nil, fmt.Errorf("invalid table name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	schema, name, err := pg.resolveTable(ctx, conn, table)
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, `
		SELECT con.conname,
		       n.nspname || '.' || c.relname,
		       ARRAY(SELECT a.attname FROM unnest(con.conkey) WITH ORDINALITY k(n, o)
		             JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.n ORDER BY k.o),
		       ARRAY(SELECT a.attname FROM unnest(con.confkey) WITH ORDINALITY k(n, o)
		             JOIN pg_attribute a ON a.attrelid = con.confrelid AND a.attnum = k.n ORDER BY k.o),
		       CASE con.confdeltype WHEN 'c' THEN 'CASCADE' WHEN 'n' THEN 'SET NULL' WHEN 'r' THEN 'RESTRICT'
		            WHEN 'd' THEN 'SET DEFAULT' ELSE 'NO ACTION' END,
		       CASE con.confupdtype WHEN 'c' THEN 'CASCADE' WHEN 'n' THEN 'SET NULL' WHEN 'r' THEN 'RESTRICT'
		            WHEN 'd' THEN 'SET DEFAULT' ELSE 'NO ACTION' END
		FROM pg_constraint con
		JOIN pg_class c ON c.oid = con.conrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_class rc ON rc.oid = con.confrelid
		JOIN pg_namespace rn ON rn.oid = rc.relnamespace
		WHERE con.contype = 'f' AND rn.nspname = $1 AND rc.relname = $2
		  AND con.conparentid = 0
		ORDER BY 2, con.conname`, schema, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ForeignKey{}
	ref := schema + "." + name
	for rows.Next() {
		fk := ForeignKey{RefTable: ref}
		if err := rows.Scan(&fk.Name, &fk.Table, &fk.Columns, &fk.RefColumns, &fk.OnDelete, &fk.OnUpdate); err != nil {
			return nil, err
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}
