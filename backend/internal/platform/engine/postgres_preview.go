package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

var _ Previewer = (*Postgres)(nil)

// PreviewBatch dry-runs a script in one transaction that is always rolled
// back. PostgreSQL rolls back DDL too, but the allowlist keeps previews to
// statements whose effects a rollback fully undoes.
func (pg *Postgres) PreviewBatch(ctx context.Context, p ConnParams, database string, stmts []string, limit int, onSession func(id int64)) ([]QueryResult, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	if err := checkBatch(stmts, true); err != nil {
		return nil, err
	}
	for i, st := range stmts {
		if err := PreviewBlocker(st); err != nil {
			return nil, fmt.Errorf("statement %d: %w", i+1, err)
		}
	}
	limit = clampQueryLimit(limit)
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	if onSession != nil {
		onSession(int64(conn.PgConn().PID()))
	}
	if _, err := conn.Exec(ctx, "BEGIN"); err != nil {
		return nil, err
	}
	defer func() { _, _ = conn.Exec(context.Background(), "ROLLBACK") }()
	if _, err := conn.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return nil, err
	}

	results := make([]QueryResult, 0, len(stmts))
	for i, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		sample, meta, before, serr := pg.previewBefore(ctx, conn, stmt)
		res, err := pg.runStatement(ctx, conn, stmt, limit, true)
		if err != nil {
			return results, &BatchError{Index: i, Err: err}
		}
		if !res.ReadOnly {
			res.Notes = append([]string{}, previewNotes...)
			if serr != nil {
				res.Notes = append(res.Notes, "No sample: "+serr.Error())
			}
		}
		if sample != nil {
			err := pg.savepoint(ctx, conn, func() error {
				return previewAfter(ctx, sample, meta, before, func(key RowValues) (RowValues, error) {
					return pgReadByKey(ctx, conn, meta, key, false)
				})
			})
			if err != nil {
				res.Notes = append(res.Notes, "No sample: "+err.Error())
			} else {
				res.Sample = sample
			}
		}
		results = append(results, *res)
	}
	return results, nil
}

// savepoint runs fn so that its failure does not abort the transaction.
func (pg *Postgres) savepoint(ctx context.Context, conn *pgx.Conn, fn func() error) error {
	if _, err := conn.Exec(ctx, "SAVEPOINT fd_preview_sample"); err != nil {
		return err
	}
	if err := fn(); err != nil {
		_, _ = conn.Exec(ctx, "ROLLBACK TO SAVEPOINT fd_preview_sample")
		return err
	}
	_, err := conn.Exec(ctx, "RELEASE SAVEPOINT fd_preview_sample")
	return err
}

// previewBefore samples the rows a single-table UPDATE / DELETE will touch.
func (pg *Postgres) previewBefore(ctx context.Context, conn *pgx.Conn, stmt string) (*PreviewSample, *tableMeta, []RowValues, error) {
	t, ok := parseSingleTableDML(stmt)
	if !ok {
		return nil, nil, nil, nil
	}
	var meta *tableMeta
	var before []RowValues
	err := pg.savepoint(ctx, conn, func() error {
		var err error
		if meta, err = pg.pgMeta(ctx, conn, strings.Join(t.parts, ".")); err != nil {
			return err
		}
		if len(meta.key) == 0 {
			meta = nil
			return nil
		}
		rows, err := conn.Query(ctx, t.sampleSQL(previewSampleRows), pgText)
		if err != nil {
			return err
		}
		defer rows.Close()
		var cols []string
		for _, fd := range rows.FieldDescriptions() {
			cols = append(cols, string(fd.Name))
		}
		for rows.Next() {
			row := textRow(rows, 0)
			v := RowValues{}
			for i, c := range cols {
				v[c] = row[i]
			}
			before = append(before, v)
		}
		return rows.Err()
	})
	if err != nil || meta == nil {
		return nil, nil, nil, err
	}
	return newSample(t, meta, &before), meta, before, nil
}
