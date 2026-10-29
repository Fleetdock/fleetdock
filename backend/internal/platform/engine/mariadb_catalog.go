package engine

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

var _ Catalog = (*MariaDB)(nil)

// Columns lists the database's tables and views with their columns from
// information_schema, which already hides objects the login cannot read.
func (m *MariaDB) Columns(ctx context.Context, p ConnParams, database string) (*ColumnCatalog, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT c.TABLE_NAME, t.TABLE_TYPE, c.COLUMN_NAME, c.COLUMN_TYPE
		FROM information_schema.COLUMNS c
		JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = c.TABLE_SCHEMA AND t.TABLE_NAME = c.TABLE_NAME
		WHERE c.TABLE_SCHEMA = ?
		ORDER BY c.TABLE_NAME, c.ORDINAL_POSITION`, database)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var b catalogBuilder
	for rows.Next() {
		var table, typ, col, colType string
		if err := rows.Scan(&table, &typ, &col, &colType); err != nil {
			return nil, err
		}
		kind := "table"
		if strings.Contains(strings.ToUpper(typ), "VIEW") {
			kind = "view"
		}
		if !b.add("", table, kind, col, colType) {
			break
		}
	}
	return b.result(), rows.Err()
}

// ReferencedBy lists the foreign keys within the database that reference
// table.
func (m *MariaDB) ReferencedBy(ctx context.Context, p ConnParams, database, table string) ([]ForeignKey, error) {
	if _, err := m.qualified(database, table); err != nil {
		return nil, err
	}
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT k.TABLE_NAME, k.CONSTRAINT_NAME, k.COLUMN_NAME, k.REFERENCED_COLUMN_NAME,
		       r.DELETE_RULE, r.UPDATE_RULE
		FROM information_schema.KEY_COLUMN_USAGE k
		JOIN information_schema.REFERENTIAL_CONSTRAINTS r
		  ON r.CONSTRAINT_SCHEMA = k.CONSTRAINT_SCHEMA AND r.CONSTRAINT_NAME = k.CONSTRAINT_NAME
		 AND r.TABLE_NAME = k.TABLE_NAME
		WHERE k.TABLE_SCHEMA = ? AND k.REFERENCED_TABLE_SCHEMA = ? AND k.REFERENCED_TABLE_NAME = ?
		ORDER BY k.TABLE_NAME, k.CONSTRAINT_NAME, k.ORDINAL_POSITION`, database, database, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIncomingFKs(rows, table)
}

// scanIncomingFKs groups (table, constraint, column, ref column, rules) rows
// into foreign keys.
func scanIncomingFKs(rows *sql.Rows, refTable string) ([]ForeignKey, error) {
	out := []ForeignKey{}
	idx := map[string]int{}
	for rows.Next() {
		var src, name, col, refCol, onDel, onUpd string
		if err := rows.Scan(&src, &name, &col, &refCol, &onDel, &onUpd); err != nil {
			return nil, err
		}
		key := src + "\x00" + name
		i, ok := idx[key]
		if !ok {
			out = append(out, ForeignKey{Name: name, Table: src, RefTable: refTable, OnDelete: onDel, OnUpdate: onUpd})
			i = len(out) - 1
			idx[key] = i
		}
		out[i].Columns = append(out[i].Columns, col)
		out[i].RefColumns = append(out[i].RefColumns, refCol)
	}
	return out, rows.Err()
}
