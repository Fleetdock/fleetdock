package engine

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
)

// pgDialect casts every parameter from text to the column's catalog type, so
// values travel as text and PostgreSQL applies its own input parsing — the
// same as typing a literal, with none of the injection risk.
var pgDialect = dialect{
	quote: quotePGIdent,
	param: func(n int, c column) string {
		t := c.Type
		if t == "" {
			t = "text"
		}
		return fmt.Sprintf("($%d::text)::%s", n, t)
	},
	text: func(q string) string { return q + "::text" },
	like: "ILIKE",
	concat: func(parts ...string) string {
		return "(" + strings.Join(parts, " || ") + ")"
	},
}

// pgMeta loads a table's columns (with exact catalog types) and row key.
// Types come from format_type() — the catalog, not the user — so they are
// safe to splice into casts.
func (pg *Postgres) pgMeta(ctx context.Context, conn *pgx.Conn, name string) (*tableMeta, error) {
	if !validTableName(name) {
		return nil, fmt.Errorf("invalid table name")
	}
	schema, table, err := pg.resolveTable(ctx, conn, name)
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, `
		SELECT a.attname, format_type(a.atttypid, a.atttypmod), NOT a.attnotnull,
		       a.atthasdef OR NOT a.attnotnull OR a.attidentity <> '' OR a.attgenerated <> '',
		       a.attgenerated <> '', a.attidentity = 'a'
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2 AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`, schema, table)
	if err != nil {
		return nil, err
	}
	var cols []column
	for rows.Next() {
		var c column
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.HasDefault, &c.Generated, &c.IdentityAlways); err != nil {
			rows.Close()
			return nil, err
		}
		cols = append(cols, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("table %q not found", name)
	}

	krows, err := conn.Query(ctx, `
		SELECT array_agg(a.attname ORDER BY k.ord)
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, ord)
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum
		WHERE n.nspname = $1 AND c.relname = $2 AND i.indisunique
		  AND i.indpred IS NULL AND i.indexprs IS NULL
		GROUP BY i.indexrelid, i.indisprimary
		ORDER BY i.indisprimary DESC, i.indexrelid`, schema, table)
	if err != nil {
		return nil, err
	}
	defer krows.Close()
	var candidates [][]string
	for krows.Next() {
		var k []string
		if err := krows.Scan(&k); err != nil {
			return nil, err
		}
		candidates = append(candidates, k)
	}
	if err := krows.Err(); err != nil {
		return nil, err
	}
	m := newTableMeta(qualifiedTable(schema, table), cols, nil)
	m.key = chooseKey(candidates, m.byName)
	return m, nil
}

// BrowseRows returns a filtered, sorted page of a table.
func (pg *Postgres) BrowseRows(ctx context.Context, p ConnParams, database string, req BrowseRequest) (*BrowseResult, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	clampBrowse(&req)
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	meta, err := pg.pgMeta(ctx, conn, req.Table)
	if err != nil {
		return nil, err
	}
	d := pgDialect
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
		q := fmt.Sprintf("SELECT count(*) FROM (SELECT 1 FROM %s%s LIMIT %d) s", meta.qualified, where, TotalCap+1)
		if err := conn.QueryRow(ctx, q, args...).Scan(&res.Total); err != nil {
			return nil, err
		}
		res.TotalExact = true
		if res.Total > TotalCap {
			res.Total, res.TotalCapped = TotalCap, true
		}
	} else {
		_ = conn.QueryRow(ctx, `SELECT GREATEST(reltuples::bigint, 0) FROM pg_class WHERE oid = $1::regclass`,
			meta.qualified).Scan(&res.Total)
	}

	rows, err := conn.Query(ctx, fmt.Sprintf("SELECT * FROM %s%s%s LIMIT %d OFFSET %d",
		meta.qualified, where, order, req.Limit, req.Offset), append([]any{pgText}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		res.Rows = append(res.Rows, textRow(rows, 4096))
	}
	return res, rows.Err()
}

// pgRowTx adapts a PostgreSQL transaction to the row-image logic.
type pgRowTx struct{ tx pgx.Tx }

func (t pgRowTx) read(ctx context.Context, m *tableMeta, key RowValues, forUpdate bool) (RowValues, error) {
	return pgReadByKey(ctx, t.tx, m, key, forUpdate)
}

// pgQueryer is what both *pgx.Conn and pgx.Tx offer.
type pgQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// pgReadByKey reads one row's full values by key, or nil if there is none.
func pgReadByKey(ctx context.Context, q pgQueryer, m *tableMeta, key RowValues, forUpdate bool) (RowValues, error) {
	sqlText, args, err := pgDialect.selectByKeySQL(m, key, forUpdate)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, sqlText, append([]any{pgText}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out RowValues
	for rows.Next() {
		if out != nil {
			return nil, exactlyOne(2)
		}
		out = m.rowToValues(textRow(rows, 0))
	}
	return out, rows.Err()
}

func (t pgRowTx) exec(ctx context.Context, q string, args []any) (int64, error) {
	tag, err := t.tx.Exec(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// insert returns the key columns as stored, which covers serial and
// identity keys the INSERT left to the database.
func (t pgRowTx) insert(ctx context.Context, m *tableMeta, q string, args []any) (int64, RowValues, error) {
	if len(m.key) == 0 {
		n, err := t.exec(ctx, q, args)
		return n, nil, err
	}
	names := make([]string, len(m.key))
	for i, k := range m.key {
		names[i] = quotePGIdent(k)
	}
	rows, err := t.tx.Query(ctx, q+" RETURNING "+strings.Join(names, ", "), append([]any{pgText}, args...)...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	var n int64
	assigned := RowValues{}
	for rows.Next() {
		n++
		for i, v := range textRow(rows, 0) {
			assigned[m.key[i]] = v
		}
	}
	return n, assigned, rows.Err()
}

// pgSessionSettings are the settings that change how values read as text.
var pgSessionSettings = []string{"TimeZone", "DateStyle", "IntervalStyle"}

func (pg *Postgres) rowTx(ctx context.Context, p ConnParams, database, table string, session map[string]string,
	fn func(tx rowTx, meta *tableMeta) (*RowChange, error)) (*RowChange, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	meta, err := pg.pgMeta(ctx, conn, table)
	if err != nil {
		return nil, err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	settings := map[string]string{}
	for _, name := range pgSessionSettings {
		if v := session[name]; v != "" {
			if _, err := tx.Exec(ctx, `SELECT set_config($1, $2, true)`, name, v); err != nil {
				return nil, err
			}
		}
		var cur string
		if err := tx.QueryRow(ctx, `SELECT current_setting($1)`, name).Scan(&cur); err != nil {
			return nil, err
		}
		settings[name] = cur
	}
	ch, err := fn(pgRowTx{tx}, meta)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	ch.Session = settings
	return ch, nil
}

// InsertRow inserts one row; omitted columns take their defaults.
func (pg *Postgres) InsertRow(ctx context.Context, p ConnParams, database, table string, values RowValues) (*RowChange, error) {
	return pg.rowTx(ctx, p, database, table, nil, func(tx rowTx, meta *tableMeta) (*RowChange, error) {
		return changeRow(ctx, pgDialect, tx, meta, "insert", nil, values)
	})
}

// UpdateRow updates the row identified by key.
func (pg *Postgres) UpdateRow(ctx context.Context, p ConnParams, database, table string, key, values RowValues) (*RowChange, error) {
	return pg.rowTx(ctx, p, database, table, nil, func(tx rowTx, meta *tableMeta) (*RowChange, error) {
		return changeRow(ctx, pgDialect, tx, meta, "update", key, values)
	})
}

// DeleteRow deletes the row identified by key.
func (pg *Postgres) DeleteRow(ctx context.Context, p ConnParams, database, table string, key RowValues) (*RowChange, error) {
	return pg.rowTx(ctx, p, database, table, nil, func(tx rowTx, meta *tableMeta) (*RowChange, error) {
		return changeRow(ctx, pgDialect, tx, meta, "delete", key, nil)
	})
}

// ApplyRowImage makes one row match an image (see ApplyRequest).
func (pg *Postgres) ApplyRowImage(ctx context.Context, p ConnParams, database, table string, req ApplyRequest) (*RowChange, error) {
	return pg.rowTx(ctx, p, database, table, req.Session, func(tx rowTx, meta *tableMeta) (*RowChange, error) {
		return applyRow(ctx, pgDialect, tx, meta, req)
	})
}

// ImportCSV loads a CSV into the table in one transaction (all or nothing).
func (pg *Postgres) ImportCSV(ctx context.Context, p ConnParams, database, table string, r io.Reader, opts ImportOptions) (int64, error) {
	if !identRe.MatchString(database) {
		return 0, fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return 0, err
	}
	defer conn.Close(ctx)
	meta, err := pg.pgMeta(ctx, conn, table)
	if err != nil {
		return 0, err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	n, err := csvBatches(meta, r, opts, func(cols []column, rows [][]*string) error {
		q, args := pgDialect.batchInsertSQL(meta, cols, rows)
		_, err := tx.Exec(ctx, q, args...)
		return err
	})
	if err != nil {
		return 0, err
	}
	return n, tx.Commit(ctx)
}
