package engine

import (
	"context"
	"database/sql"
	"fmt"
)

var _ RowReader = (*MariaDB)(nil)

func (m *MariaDB) readRows(ctx context.Context, p ConnParams, database, table string,
	build func(meta *tableMeta) (string, []any, error), limit int) (*ScanPage, error) {
	conn, closeSession, err := m.session(ctx, p)
	if err != nil {
		return nil, err
	}
	defer closeSession()
	if _, err := conn.ExecContext(ctx, "SET time_zone = '+00:00'"); err != nil {
		return nil, err
	}
	meta, err := mysqlMeta(ctx, conn, database, table)
	if err != nil {
		return nil, err
	}
	q, args, err := build(meta)
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanAll(rows, meta)
	if err != nil {
		return nil, err
	}
	return meta.page(out, limit > 0 && len(out) == limit), nil
}

func scanAll(rows *sql.Rows, meta *tableMeta) ([]RowValues, error) {
	var out []RowValues
	for rows.Next() {
		row, err := scanStringRow(rows, len(meta.columns), 0)
		if err != nil {
			return nil, err
		}
		out = append(out, meta.rowToValues(row))
	}
	return out, rows.Err()
}

// ScanRows returns the next page of rows in key order.
func (m *MariaDB) ScanRows(ctx context.Context, p ConnParams, database, table string, after RowValues, limit int) (*ScanPage, error) {
	return m.readRows(ctx, p, database, table, func(meta *tableMeta) (string, []any, error) {
		return mysqlDialect.scanSQL(meta, after, limit)
	}, limit)
}

// GetRows returns the rows with the given keys.
func (m *MariaDB) GetRows(ctx context.Context, p ConnParams, database, table string, keys []RowValues) (*ScanPage, error) {
	return m.readRows(ctx, p, database, table, func(meta *tableMeta) (string, []any, error) {
		return mysqlDialect.getSQL(meta, keys)
	}, 0)
}

// DisableEvents disables every scheduled event of a database (a restored
// backup copy's events would otherwise keep running, possibly writing to
// other databases). It returns how many it disabled.
func (m *MariaDB) DisableEvents(ctx context.Context, p ConnParams, database string) (int, error) {
	if !identRe.MatchString(database) {
		return 0, fmt.Errorf("invalid database name")
	}
	db, err := m.open(p)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT EVENT_NAME FROM information_schema.EVENTS WHERE EVENT_SCHEMA = ? AND STATUS = 'ENABLED'`, database)
	if err != nil {
		return 0, err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			_ = rows.Close()
			return 0, err
		}
		names = append(names, n)
	}
	_ = rows.Close()
	for _, n := range names {
		if _, err := db.ExecContext(ctx, "ALTER EVENT "+quoteMySQLIdent(database)+"."+quoteMySQLIdent(n)+" DISABLE"); err != nil {
			return 0, err
		}
	}
	return len(names), nil
}
