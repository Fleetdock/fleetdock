package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

var _ Previewer = (*MariaDB)(nil)

// PreviewBatch dry-runs a script: autocommit off and one transaction, so
// even a deadlock that rolls the transaction back cannot leave later
// statements committed; refused when the database has tables whose engine
// cannot roll back (MyISAM, Aria, MEMORY…).
func (m *MariaDB) PreviewBatch(ctx context.Context, p ConnParams, database string, stmts []string, limit int, onSession func(id int64)) ([]QueryResult, error) {
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
	p.Database = database
	conn, closeSession, err := m.session(ctx, p)
	if err != nil {
		return nil, err
	}
	defer closeSession()
	if onSession != nil {
		var id int64
		if err := conn.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&id); err == nil {
			onSession(id)
		}
	}
	if err := mysqlNonTransactional(ctx, conn, database); err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "SET SESSION autocommit = 0"); err != nil {
		return nil, err
	}
	_, _ = conn.ExecContext(ctx, "SET SESSION innodb_lock_wait_timeout = 5")
	if _, err := conn.ExecContext(ctx, "START TRANSACTION"); err != nil {
		return nil, err
	}
	rolledBack := false
	defer func() {
		if !rolledBack {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	results := make([]QueryResult, 0, len(stmts))
	for i, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		sample, meta, before, serr := m.previewBefore(ctx, conn, database, stmt)
		if serr != nil && isLockFailure(serr) {
			return results, &BatchError{Index: i, Err: serr}
		}
		res, err := m.runStatement(ctx, conn, stmt, limit, true)
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
			if err := previewAfter(ctx, sample, meta, before, func(key RowValues) (RowValues, error) {
				return mysqlReadByKey(ctx, conn, meta, key, false)
			}); err != nil {
				if isLockFailure(err) {
					return results, &BatchError{Index: i, Err: err}
				}
				res.Notes = append(res.Notes, "No sample: "+err.Error())
			} else {
				res.Sample = sample
			}
		}
		results = append(results, *res)
	}
	if _, err := conn.ExecContext(ctx, "ROLLBACK"); err != nil {
		return results, err
	}
	rolledBack = true
	// 1196: "Some non-transactional changed tables couldn't be rolled back"
	// (a trigger or function wrote to a MyISAM table elsewhere).
	if w, err := conn.QueryContext(ctx, "SHOW WARNINGS"); err == nil {
		for w.Next() {
			var level, msg string
			var code int
			if w.Scan(&level, &code, &msg) == nil && code == 1196 && len(results) > 0 {
				last := &results[len(results)-1]
				last.Notes = append(last.Notes, "WARNING: "+msg+" — part of this preview was really written.")
			}
		}
		_ = w.Close()
	}
	return results, nil
}

// mysqlNonTransactional refuses a preview when a table of the database
// cannot roll back.
func mysqlNonTransactional(ctx context.Context, q queryer, database string) error {
	rows, err := q.QueryContext(ctx, `
		SELECT TABLE_NAME, ENGINE FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE' AND ENGINE IS NOT NULL
		  AND UPPER(ENGINE) NOT IN ('INNODB', 'ROCKSDB', 'TOKUDB')
		ORDER BY TABLE_NAME LIMIT 6`, database)
	if err != nil {
		return err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var t, e string
		if err := rows.Scan(&t, &e); err != nil {
			return err
		}
		names = append(names, fmt.Sprintf("%s (%s)", t, e))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(names) > 0 {
		more := ""
		if len(names) > 5 {
			names, more = names[:5], ", …"
		}
		return fmt.Errorf("preview needs every table to roll back, but these use an engine that cannot: %s%s", strings.Join(names, ", "), more)
	}
	return nil
}

// previewBefore samples the rows a single-table UPDATE / DELETE will touch.
// It returns nil, nil when the statement is not one it understands.
func (m *MariaDB) previewBefore(ctx context.Context, conn *sql.Conn, database, stmt string) (*PreviewSample, *tableMeta, []RowValues, error) {
	t, ok := parseSingleTableDML(stmt)
	if !ok {
		return nil, nil, nil, nil
	}
	if len(t.parts) == 2 && t.parts[0] != database {
		return nil, nil, nil, nil
	}
	meta, err := mysqlMeta(ctx, conn, database, t.parts[len(t.parts)-1])
	if err != nil || len(meta.key) == 0 {
		return nil, nil, nil, err
	}
	rows, err := conn.QueryContext(ctx, t.sampleSQL(previewSampleRows))
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, nil, err
	}
	var before []RowValues
	for rows.Next() {
		row, err := scanStringRow(rows, len(cols), 0)
		if err != nil {
			return nil, nil, nil, err
		}
		v := RowValues{}
		for i, c := range cols {
			v[c] = row[i]
		}
		before = append(before, v)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	return newSample(t, meta, &before), meta, before, nil
}

// isLockFailure: a deadlock (1213) or lock wait timeout (1205), after which
// the preview must stop.
func isLockFailure(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && (me.Number == 1213 || me.Number == 1205)
}
