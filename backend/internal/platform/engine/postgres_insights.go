package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var _ Insights = (*Postgres)(nil)

// IndexReport lists the database's indexes with pg_stat_user_indexes scan
// counts, which PostgreSQL always collects.
func (pg *Postgres) IndexReport(ctx context.Context, p ConnParams, database string) (*IndexReport, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `
		SELECT n.nspname, t.relname, i.relname, ix.indisunique, ix.indisprimary,
		       ix.indpred IS NOT NULL, ix.indexprs IS NOT NULL, am.amname,
		       ARRAY(SELECT coalesce(a.attname, '') FROM unnest(ix.indkey[0:ix.indnkeyatts - 1]) WITH ORDINALITY k(n, o)
		             LEFT JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.n ORDER BY k.o),
		       pg_relation_size(i.oid), s.idx_scan
		FROM pg_index ix
		JOIN pg_class i ON i.oid = ix.indexrelid
		JOIN pg_class t ON t.oid = ix.indrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN pg_am am ON am.oid = i.relam
		LEFT JOIN pg_stat_user_indexes s ON s.indexrelid = i.oid
		WHERE `+pgUserSchema("n.nspname")+`
		ORDER BY n.nspname, t.relname, i.relname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rep := &IndexReport{Indexes: []IndexDef{}, UsageAvailable: true}
	for rows.Next() {
		var ix IndexDef
		var cols []string
		var size int64
		var scans *int64
		if err := rows.Scan(&ix.Schema, &ix.Table, &ix.Name, &ix.Unique, &ix.Primary, &ix.Partial, &ix.Expression,
			&ix.Method, &cols, &size, &scans); err != nil {
			return nil, err
		}
		ix.Columns = []string{}
		for _, c := range cols {
			if c == "" {
				ix.Expression = true // attnum 0: an expression key
				continue
			}
			ix.Columns = append(ix.Columns, c)
		}
		ix.SizeBytes, ix.Scans = &size, scans
		rep.Indexes = append(rep.Indexes, ix)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var since time.Time
	if err := conn.QueryRow(ctx, `
		SELECT coalesce(d.stats_reset, pg_postmaster_start_time())
		FROM pg_stat_database d WHERE d.datname = current_database()`).Scan(&since); err == nil {
		since = since.UTC()
		rep.StatsSince = &since
	}
	return rep, nil
}

const pgStatStatementsHelp = "Statement statistics need the pg_stat_statements extension: add it to " +
	"shared_preload_libraries in postgresql.conf, restart, then run CREATE EXTENSION pg_stat_statements " +
	"in the postgres database."

// TopQueries reads pg_stat_statements. The view is cluster-wide but exists
// only in databases where the extension was created, so the maintenance
// database is tried first, then the requested one.
func (pg *Postgres) TopQueries(ctx context.Context, p ConnParams, req TopQueriesRequest) (*TopQueries, error) {
	req = req.normalized()
	out := &TopQueries{Queries: []TopQuery{}}
	candidates := []string{"postgres"}
	if req.Database != "" && req.Database != "postgres" && identRe.MatchString(req.Database) {
		candidates = append(candidates, req.Database)
	}
	for _, dbName := range candidates {
		conn, err := pg.connect(ctx, p, dbName)
		if err != nil {
			return nil, err
		}
		done, err := pg.topQueries(ctx, conn, req, out)
		conn.Close(ctx)
		if err != nil {
			return nil, err
		}
		if done {
			return out, nil
		}
	}
	if out.Note == "" {
		out.Note = pgStatStatementsHelp
	}
	return out, nil
}

// topQueries fills out from conn's pg_stat_statements; false when the
// extension is not installed in that database.
func (pg *Postgres) topQueries(ctx context.Context, conn *pgx.Conn, req TopQueriesRequest, out *TopQueries) (bool, error) {
	var installed bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')`).Scan(&installed); err != nil {
		return false, err
	}
	if !installed {
		return false, nil
	}
	order := map[string]string{"total": "s.total_exec_time", "mean": "s.mean_exec_time", "calls": "s.calls"}[req.Sort]
	q := `
		SELECT coalesce(d.datname, ''), s.query, s.calls, s.total_exec_time, s.mean_exec_time, s.rows,
		       CASE WHEN s.shared_blks_hit + s.shared_blks_read > 0
		            THEN 100.0 * s.shared_blks_hit / (s.shared_blks_hit + s.shared_blks_read) END
		FROM pg_stat_statements s
		LEFT JOIN pg_database d ON d.oid = s.dbid`
	args := []any{}
	if req.Database != "" {
		q += ` WHERE d.datname = $1`
		args = append(args, req.Database)
	}
	q += fmt.Sprintf(` ORDER BY %s DESC LIMIT %d`, order, req.Limit)
	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		// Typically "must be loaded via shared_preload_libraries".
		out.Note = pgStatStatementsHelp + " (" + err.Error() + ")"
		return true, nil
	}
	defer rows.Close()
	for rows.Next() {
		var t TopQuery
		if err := rows.Scan(&t.Database, &t.Query, &t.Calls, &t.TotalMS, &t.MeanMS, &t.Rows, &t.CacheHitPct); err != nil {
			return false, err
		}
		out.Queries = append(out.Queries, t)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	out.Available = true
	var since time.Time
	if conn.QueryRow(ctx, `SELECT stats_reset FROM pg_stat_statements_info`).Scan(&since) == nil {
		since = since.UTC()
		out.StatsSince = &since
	}
	return true, nil
}

// Explain runs EXPLAIN (FORMAT JSON). With analyze it runs EXPLAIN (ANALYZE,
// BUFFERS) inside a read-only transaction that is rolled back.
func (pg *Postgres) Explain(ctx context.Context, p ConnParams, database, stmt string, analyze bool) (*ExplainResult, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	stmt, err := explainable(stmt)
	if err != nil {
		return nil, err
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	opts := "FORMAT JSON"
	if analyze {
		opts = "ANALYZE, BUFFERS, FORMAT JSON"
		if _, err := conn.Exec(ctx, "BEGIN READ ONLY"); err != nil {
			return nil, err
		}
		defer func() { _, _ = conn.Exec(context.Background(), "ROLLBACK") }()
	}
	var plan string
	if err := conn.QueryRow(ctx, "EXPLAIN ("+opts+") "+strings.TrimSpace(stmt), pgText).Scan(&plan); err != nil {
		return nil, err
	}
	return &ExplainResult{Format: "postgres", Analyzed: analyze, Plan: json.RawMessage(plan)}, nil
}
