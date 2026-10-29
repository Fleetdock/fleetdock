package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

var _ Insights = (*MariaDB)(nil)

// isMariaDB tells MariaDB from MySQL by the server version string.
func isMariaDB(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) (bool, error) {
	var v string
	if err := q.QueryRowContext(ctx, "SELECT VERSION()").Scan(&v); err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(v), "mariadb"), nil
}

// statsSince is when the server's counters started: its start time.
func mysqlStatsSince(ctx context.Context, db *sql.DB) *time.Time {
	var name string
	var uptime int64
	if err := db.QueryRowContext(ctx, "SHOW GLOBAL STATUS LIKE 'Uptime'").Scan(&name, &uptime); err != nil {
		return nil
	}
	t := time.Now().Add(-time.Duration(uptime) * time.Second).UTC().Truncate(time.Second)
	return &t
}

// IndexReport lists indexes from information_schema.STATISTICS. Usage comes
// from performance_schema (MySQL's default) or, on MariaDB where that is off
// by default, from the user-statistics plugin's INDEX_STATISTICS when
// userstat=1; otherwise usage is reported unavailable.
func (m *MariaDB) IndexReport(ctx context.Context, p ConnParams, database string) (*IndexReport, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, COLUMN_NAME, SUB_PART, INDEX_TYPE
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ?
		ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`, database)
	if err != nil {
		return nil, err
	}
	rep := &IndexReport{Indexes: []IndexDef{}}
	pos := map[string]int{}
	for rows.Next() {
		var table, name, typ string
		var nonUnique int
		var col sql.NullString
		var subPart sql.NullInt64
		if err := rows.Scan(&table, &name, &nonUnique, &col, &subPart, &typ); err != nil {
			rows.Close()
			return nil, err
		}
		k := table + "\x00" + name
		i, ok := pos[k]
		if !ok {
			rep.Indexes = append(rep.Indexes, IndexDef{
				Table: table, Name: name, Unique: nonUnique == 0, Primary: name == "PRIMARY", Method: typ, Columns: []string{},
			})
			i = len(rep.Indexes) - 1
			pos[k] = i
		}
		ix := &rep.Indexes[i]
		switch {
		case !col.Valid:
			ix.Expression = true // MySQL 8 functional key part
		case subPart.Valid:
			ix.Columns = append(ix.Columns, fmt.Sprintf("%s(%d)", col.String, subPart.Int64))
		default:
			ix.Columns = append(ix.Columns, col.String)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// InnoDB index sizes (best effort: needs read access to mysql.*).
	if srows, err := db.QueryContext(ctx, `
		SELECT s.table_name, s.index_name, s.stat_value * @@innodb_page_size
		FROM mysql.innodb_index_stats s
		WHERE s.database_name = ? AND s.stat_name = 'size'`, database); err == nil {
		for srows.Next() {
			var table, name string
			var size int64
			if srows.Scan(&table, &name, &size) == nil {
				if i, ok := pos[table+"\x00"+name]; ok {
					rep.Indexes[i].SizeBytes = &size
				}
			}
		}
		srows.Close()
	}

	maria, err := isMariaDB(ctx, db)
	if err != nil {
		return nil, err
	}
	var perfSchema int
	_ = db.QueryRowContext(ctx, "SELECT @@performance_schema").Scan(&perfSchema)
	switch {
	case perfSchema == 1:
		urows, err := db.QueryContext(ctx, `
			SELECT OBJECT_NAME, INDEX_NAME, COUNT_STAR
			FROM performance_schema.table_io_waits_summary_by_index_usage
			WHERE OBJECT_SCHEMA = ? AND INDEX_NAME IS NOT NULL`, database)
		if err != nil {
			rep.Note = "index usage could not be read from performance_schema: " + err.Error()
			return rep, nil
		}
		for i := range rep.Indexes {
			zero := int64(0)
			rep.Indexes[i].Scans = &zero
		}
		for urows.Next() {
			var table, name string
			var n int64
			if urows.Scan(&table, &name, &n) == nil {
				if i, ok := pos[table+"\x00"+name]; ok {
					rep.Indexes[i].Scans = &n
				}
			}
		}
		urows.Close()
		rep.UsageAvailable = true
	case maria:
		var userstat int
		_ = db.QueryRowContext(ctx, "SELECT @@userstat").Scan(&userstat)
		if userstat != 1 {
			rep.Note = "MariaDB is not collecting index usage. Set performance_schema=ON in the server " +
				"configuration (needs a restart), or run SET GLOBAL userstat=1 to start counting now."
			return rep, nil
		}
		for i := range rep.Indexes {
			zero := int64(0)
			rep.Indexes[i].Scans = &zero
		}
		urows, err := db.QueryContext(ctx, `
			SELECT TABLE_NAME, INDEX_NAME, ROWS_READ FROM information_schema.INDEX_STATISTICS
			WHERE TABLE_SCHEMA = ?`, database)
		if err != nil {
			return nil, err
		}
		for urows.Next() {
			var table, name string
			var n int64
			if urows.Scan(&table, &name, &n) == nil {
				if i, ok := pos[table+"\x00"+name]; ok {
					rep.Indexes[i].Scans = &n
				}
			}
		}
		urows.Close()
		rep.UsageAvailable = true
		rep.Note = "Counted by userstat since it was switched on or the statistics were flushed."
	default:
		rep.Note = "MySQL is not collecting index usage: performance_schema is OFF. Enable it in the server " +
			"configuration (needs a restart)."
		return rep, nil
	}
	rep.StatsSince = mysqlStatsSince(ctx, db)
	return rep, nil
}

// TopQueries reads performance_schema's statement digests: normalized
// statements with their call counts and timers (picoseconds).
func (m *MariaDB) TopQueries(ctx context.Context, p ConnParams, req TopQueriesRequest) (*TopQueries, error) {
	req = req.normalized()
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	out := &TopQueries{Queries: []TopQuery{}}
	var perfSchema int
	_ = db.QueryRowContext(ctx, "SELECT @@performance_schema").Scan(&perfSchema)
	if perfSchema != 1 {
		out.Note = "Statement statistics need performance_schema=ON in the server configuration (needs a restart)."
		return out, nil
	}
	order := map[string]string{"total": "SUM_TIMER_WAIT", "mean": "AVG_TIMER_WAIT", "calls": "COUNT_STAR"}[req.Sort]
	q := `
		SELECT COALESCE(SCHEMA_NAME, ''), DIGEST_TEXT, COUNT_STAR, SUM_TIMER_WAIT / 1e9, AVG_TIMER_WAIT / 1e9,
		       SUM_ROWS_SENT + SUM_ROWS_AFFECTED, SUM_ROWS_EXAMINED, SUM_NO_INDEX_USED + SUM_NO_GOOD_INDEX_USED
		FROM performance_schema.events_statements_summary_by_digest
		WHERE DIGEST_TEXT IS NOT NULL`
	args := []any{}
	if req.Database != "" {
		q += ` AND SCHEMA_NAME = ?`
		args = append(args, req.Database)
	}
	q += ` ORDER BY ` + order + ` DESC LIMIT ?`
	args = append(args, req.Limit)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		out.Note = "Statement statistics could not be read: " + err.Error()
		return out, nil
	}
	defer rows.Close()
	for rows.Next() {
		var t TopQuery
		var examined, noIndex int64
		if err := rows.Scan(&t.Database, &t.Query, &t.Calls, &t.TotalMS, &t.MeanMS, &t.Rows, &examined, &noIndex); err != nil {
			return nil, err
		}
		t.RowsExamined, t.NoIndexUsed = &examined, &noIndex
		out.Queries = append(out.Queries, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out.Available = true
	out.StatsSince = mysqlStatsSince(ctx, db)
	return out, nil
}

// Explain runs EXPLAIN FORMAT=JSON. With analyze, MariaDB's ANALYZE
// FORMAT=JSON or MySQL's EXPLAIN ANALYZE (tree text) executes the statement
// inside a read-only transaction that is rolled back.
func (m *MariaDB) Explain(ctx context.Context, p ConnParams, database, stmt string, analyze bool) (*ExplainResult, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	stmt, err := explainable(stmt)
	if err != nil {
		return nil, err
	}
	p.Database = database
	conn, done, err := m.session(ctx, p)
	if err != nil {
		return nil, err
	}
	defer done()
	maria, err := isMariaDB(ctx, conn)
	if err != nil {
		return nil, err
	}
	format := "mysql"
	if maria {
		format = "mariadb"
	}
	if !analyze {
		var plan string
		if err := conn.QueryRowContext(ctx, "EXPLAIN FORMAT=JSON "+stmt).Scan(&plan); err != nil {
			return nil, err
		}
		return &ExplainResult{Format: format, Plan: json.RawMessage(plan)}, nil
	}
	if _, err := conn.ExecContext(ctx, "START TRANSACTION READ ONLY"); err != nil {
		return nil, err
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "ROLLBACK") }()
	if maria {
		var plan string
		if err := conn.QueryRowContext(ctx, "ANALYZE FORMAT=JSON "+stmt).Scan(&plan); err != nil {
			return nil, err
		}
		return &ExplainResult{Format: format, Analyzed: true, Plan: json.RawMessage(plan)}, nil
	}
	var text string
	if err := conn.QueryRowContext(ctx, "EXPLAIN ANALYZE "+stmt).Scan(&text); err != nil {
		return nil, err
	}
	return &ExplainResult{Format: "mysql-tree", Analyzed: true, Text: text}, nil
}
