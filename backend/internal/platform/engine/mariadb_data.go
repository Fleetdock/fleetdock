package engine

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
)

func quoteMySQLIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

var mysqlDialect = dialect{
	quote: quoteMySQLIdent,
	param: func(int, column) string { return "?" },
	text:  func(q string) string { return "CAST(" + q + " AS CHAR)" },
	like:  "LIKE",
	concat: func(parts ...string) string {
		return "CONCAT(" + strings.Join(parts, ", ") + ")"
	},
	positional: true,
}

// queryer is what both *sql.Conn and *sql.Tx offer.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// mysqlMeta loads a table's columns and row key from information_schema.
func mysqlMeta(ctx context.Context, q queryer, database, table string) (*tableMeta, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	if !validTableName(table) {
		return nil, fmt.Errorf("invalid table name")
	}
	rows, err := q.QueryContext(ctx, `
		SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE,
		       COLUMN_DEFAULT IS NOT NULL OR IS_NULLABLE = 'YES' OR EXTRA LIKE '%auto_increment%'
		       OR EXTRA LIKE '%GENERATED%'
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION`, database, table)
	if err != nil {
		return nil, err
	}
	var cols []column
	for rows.Next() {
		var c column
		var nullable string
		if err := rows.Scan(&c.Name, &c.Type, &nullable, &c.HasDefault); err != nil {
			_ = rows.Close()
			return nil, err
		}
		c.Nullable = nullable == "YES"
		cols = append(cols, c)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("table %q not found", table)
	}

	krows, err := q.QueryContext(ctx, `
		SELECT INDEX_NAME, COLUMN_NAME
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND NON_UNIQUE = 0
		ORDER BY INDEX_NAME = 'PRIMARY' DESC, INDEX_NAME, SEQ_IN_INDEX`, database, table)
	if err != nil {
		return nil, err
	}
	defer krows.Close()
	var order []string
	keys := map[string][]string{}
	for krows.Next() {
		var idx, col string
		if err := krows.Scan(&idx, &col); err != nil {
			return nil, err
		}
		if _, ok := keys[idx]; !ok {
			order = append(order, idx)
		}
		keys[idx] = append(keys[idx], col)
	}
	if err := krows.Err(); err != nil {
		return nil, err
	}
	candidates := make([][]string, len(order))
	for i, idx := range order {
		candidates[i] = keys[idx]
	}
	m := newTableMeta(quoteMySQLIdent(database)+"."+quoteMySQLIdent(table), cols, nil)
	m.key = chooseKey(candidates, m.byName)
	return m, nil
}

// BrowseRows returns a filtered, sorted page of a table.
func (m *MariaDB) BrowseRows(ctx context.Context, p ConnParams, database string, req BrowseRequest) (*BrowseResult, error) {
	clampBrowse(&req)
	conn, closeSession, err := m.session(ctx, p)
	if err != nil {
		return nil, err
	}
	defer closeSession()

	meta, err := mysqlMeta(ctx, conn, database, req.Table)
	if err != nil {
		return nil, err
	}
	d := mysqlDialect
	where, args, err := d.where(meta, req, nil)
	if err != nil {
		return nil, err
	}
	order, err := d.orderBy(meta, req.Sort)
	if err != nil {
		return nil, err
	}

	res := &BrowseResult{Columns: browseColumns(meta), Rows: [][]*string{}, Key: meta.key}
	if res.Key == nil {
		res.Key = []string{}
	}
	if filtered(req) {
		q := fmt.Sprintf("SELECT COUNT(*) FROM (SELECT 1 FROM %s%s LIMIT %d) s", meta.qualified, where, TotalCap+1)
		if err := conn.QueryRowContext(ctx, q, args...).Scan(&res.Total); err != nil {
			return nil, err
		}
		res.TotalExact = true
		if res.Total > TotalCap {
			res.Total, res.TotalCapped = TotalCap, true
		}
	} else {
		_ = conn.QueryRowContext(ctx, `
			SELECT COALESCE(TABLE_ROWS, 0) FROM information_schema.TABLES
			WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`, database, req.Table).Scan(&res.Total)
	}

	rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s%s%s LIMIT %d OFFSET %d",
		meta.qualified, where, order, req.Limit, req.Offset), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		row, err := scanStringRow(rows, len(cols), 4096)
		if err != nil {
			return nil, err
		}
		res.Rows = append(res.Rows, row)
	}
	return res, rows.Err()
}

// mutate runs one single-row statement in a transaction and requires it to
// affect exactly one row.
func (m *MariaDB) mutate(ctx context.Context, p ConnParams, database, table string,
	build func(meta *tableMeta) (string, []any, error), mustBeOne bool) error {
	conn, closeSession, err := m.session(ctx, p)
	if err != nil {
		return err
	}
	defer closeSession()
	meta, err := mysqlMeta(ctx, conn, database, table)
	if err != nil {
		return err
	}
	q, args, err := build(meta)
	if err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if mustBeOne {
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if err := exactlyOne(n); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// InsertRow inserts one row; omitted columns take their defaults.
func (m *MariaDB) InsertRow(ctx context.Context, p ConnParams, database, table string, values RowValues) error {
	return m.mutate(ctx, p, database, table, func(meta *tableMeta) (string, []any, error) {
		return mysqlDialect.insertSQL(meta, values)
	}, true)
}

// UpdateRow updates the row identified by key.
func (m *MariaDB) UpdateRow(ctx context.Context, p ConnParams, database, table string, key, values RowValues) error {
	return m.mutate(ctx, p, database, table, func(meta *tableMeta) (string, []any, error) {
		return mysqlDialect.updateSQL(meta, key, values)
	}, true)
}

// DeleteRow deletes the row identified by key.
func (m *MariaDB) DeleteRow(ctx context.Context, p ConnParams, database, table string, key RowValues) error {
	return m.mutate(ctx, p, database, table, func(meta *tableMeta) (string, []any, error) {
		return mysqlDialect.deleteSQL(meta, key)
	}, true)
}

// ImportCSV loads a CSV into the table in one transaction (all or nothing).
func (m *MariaDB) ImportCSV(ctx context.Context, p ConnParams, database, table string, r io.Reader, opts ImportOptions) (int64, error) {
	conn, closeSession, err := m.session(ctx, p)
	if err != nil {
		return 0, err
	}
	defer closeSession()
	meta, err := mysqlMeta(ctx, conn, database, table)
	if err != nil {
		return 0, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	n, err := csvBatches(meta, r, opts, func(cols []column, rows [][]*string) error {
		q, args := mysqlDialect.batchInsertSQL(meta, cols, rows)
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	})
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}
