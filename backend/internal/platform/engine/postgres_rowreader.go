package engine

import (
	"context"
	"fmt"
)

var _ RowReader = (*Postgres)(nil)

func (pg *Postgres) readRows(ctx context.Context, p ConnParams, database, table string,
	build func(meta *tableMeta) (string, []any, error), limit int) (*ScanPage, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	for name, v := range CanonicalSession("postgres") {
		if _, err := conn.Exec(ctx, `SELECT set_config($1, $2, false)`, name, v); err != nil {
			return nil, err
		}
	}
	meta, err := pg.pgMeta(ctx, conn, table)
	if err != nil {
		return nil, err
	}
	q, args, err := build(meta)
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, q, append([]any{pgText}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RowValues
	for rows.Next() {
		out = append(out, meta.rowToValues(textRow(rows, 0)))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return meta.page(out, limit > 0 && len(out) == limit), nil
}

// ScanRows returns the next page of rows in key order.
func (pg *Postgres) ScanRows(ctx context.Context, p ConnParams, database, table string, after RowValues, limit int) (*ScanPage, error) {
	return pg.readRows(ctx, p, database, table, func(meta *tableMeta) (string, []any, error) {
		return pgDialect.scanSQL(meta, after, limit)
	}, limit)
}

// GetRows returns the rows with the given keys.
func (pg *Postgres) GetRows(ctx context.Context, p ConnParams, database, table string, keys []RowValues) (*ScanPage, error) {
	return pg.readRows(ctx, p, database, table, func(meta *tableMeta) (string, []any, error) {
		return pgDialect.getSQL(meta, keys)
	}, 0)
}
