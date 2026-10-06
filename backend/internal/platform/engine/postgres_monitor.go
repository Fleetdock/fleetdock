package engine

import (
	"context"
	"fmt"
)

// Processes lists backends from pg_stat_activity (client sessions only),
// excluding the one running this query.
func (pg *Postgres) Processes(ctx context.Context, p ConnParams) ([]Process, error) {
	conn, err := pg.connect(ctx, p, "postgres")
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `
		SELECT pid, COALESCE(usename, ''), COALESCE(client_addr::text, ''), datname,
		       COALESCE(state, ''),
		       COALESCE(EXTRACT(EPOCH FROM now() - COALESCE(query_start, state_change, backend_start))::bigint, 0),
		       LEFT(query, $1)
		FROM pg_stat_activity
		WHERE backend_type = 'client backend' AND pid <> pg_backend_pid()
		ORDER BY 6 DESC`, maxProcessQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Process{}
	for rows.Next() {
		var pr Process
		if err := rows.Scan(&pr.ID, &pr.User, &pr.Host, &pr.Database, &pr.State, &pr.Seconds, &pr.Query); err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// KillProcess runs pg_cancel_backend (statement) or pg_terminate_backend
// (session).
func (pg *Postgres) KillProcess(ctx context.Context, p ConnParams, id int64, connection bool) error {
	if id <= 0 {
		return fmt.Errorf("invalid process id")
	}
	conn, err := pg.connect(ctx, p, "postgres")
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	fn := "pg_cancel_backend"
	if connection {
		fn = "pg_terminate_backend"
	}
	var ok bool
	if err := conn.QueryRow(ctx, "SELECT "+fn+"($1)", id).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no session with id %d", id)
	}
	return nil
}

// ServerStatus summarises instance-wide counters.
func (pg *Postgres) ServerStatus(ctx context.Context, p ConnParams) ([]Setting, error) {
	conn, err := pg.connect(ctx, p, "postgres")
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `
		SELECT k, v FROM (
		  SELECT 'version' AS k, version() AS v
		  UNION ALL SELECT 'uptime', (now() - pg_postmaster_start_time())::text
		  UNION ALL SELECT 'connections', count(*)::text FROM pg_stat_activity WHERE backend_type = 'client backend'
		  UNION ALL SELECT 'max_connections', current_setting('max_connections')
		  UNION ALL SELECT 'transactions_committed', sum(xact_commit)::text FROM pg_stat_database
		  UNION ALL SELECT 'transactions_rolled_back', sum(xact_rollback)::text FROM pg_stat_database
		  UNION ALL SELECT 'blocks_read', sum(blks_read)::text FROM pg_stat_database
		  UNION ALL SELECT 'blocks_hit', sum(blks_hit)::text FROM pg_stat_database
		  UNION ALL SELECT 'cache_hit_ratio',
		    round(100.0 * sum(blks_hit) / NULLIF(sum(blks_hit) + sum(blks_read), 0), 2)::text FROM pg_stat_database
		  UNION ALL SELECT 'deadlocks', sum(deadlocks)::text FROM pg_stat_database
		  UNION ALL SELECT 'temp_bytes', sum(temp_bytes)::text FROM pg_stat_database
		  UNION ALL SELECT 'is_in_recovery', pg_is_in_recovery()::text
		) s`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Setting{}
	for rows.Next() {
		var s Setting
		var v *string
		if err := rows.Scan(&s.Name, &v); err != nil {
			return nil, err
		}
		if v != nil {
			s.Value = *v
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Variables returns pg_settings (with units folded into the value).
func (pg *Postgres) Variables(ctx context.Context, p ConnParams) ([]Setting, error) {
	conn, err := pg.connect(ctx, p, "postgres")
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `
		SELECT name, setting || COALESCE(' ' || unit, '') FROM pg_settings ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Setting{}
	for rows.Next() {
		var s Setting
		if err := rows.Scan(&s.Name, &s.Value); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// DatabaseStats returns pg_database_size and backend count per database.
func (pg *Postgres) DatabaseStats(ctx context.Context, p ConnParams) ([]DatabaseStat, error) {
	conn, err := pg.connect(ctx, p, "postgres")
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `
		SELECT d.datname, pg_database_size(d.oid),
		       (SELECT count(*) FROM pg_stat_activity a WHERE a.datid = d.oid)::int
		FROM pg_database d
		WHERE NOT d.datistemplate AND has_database_privilege(d.oid, 'CONNECT')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DatabaseStat{}
	for rows.Next() {
		var st DatabaseStat
		if err := rows.Scan(&st.Name, &st.SizeBytes, &st.Connections); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
