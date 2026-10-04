package engine

import (
	"context"
	"database/sql"
	"fmt"
)

// Processes lists sessions from information_schema.PROCESSLIST, excluding the
// one running this query.
func (m *MariaDB) Processes(ctx context.Context, p ConnParams) ([]Process, error) {
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT ID, USER, HOST, DB, COMMAND, TIME, LEFT(INFO, ?)
		FROM information_schema.PROCESSLIST
		WHERE ID <> CONNECTION_ID()
		ORDER BY TIME DESC`, maxProcessQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Process{}
	for rows.Next() {
		var pr Process
		var db, info sql.NullString
		var user, host, cmd sql.NullString
		if err := rows.Scan(&pr.ID, &user, &host, &db, &cmd, &pr.Seconds, &info); err != nil {
			return nil, err
		}
		pr.User, pr.Host, pr.State = user.String, host.String, cmd.String
		if db.Valid {
			pr.Database = &db.String
		}
		if info.Valid {
			pr.Query = &info.String
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// KillProcess runs KILL QUERY (statement) or KILL CONNECTION (session).
func (m *MariaDB) KillProcess(ctx context.Context, p ConnParams, id int64, connection bool) error {
	if id <= 0 {
		return fmt.Errorf("invalid process id")
	}
	db, err := m.open(p)
	if err != nil {
		return err
	}
	defer db.Close()
	verb := "KILL QUERY"
	if connection {
		verb = "KILL CONNECTION"
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("%s %d", verb, id))
	return err
}

// ServerStatus returns SHOW GLOBAL STATUS.
func (m *MariaDB) ServerStatus(ctx context.Context, p ConnParams) ([]Setting, error) {
	return m.settings(ctx, p, "SHOW GLOBAL STATUS")
}

// Variables returns SHOW GLOBAL VARIABLES.
func (m *MariaDB) Variables(ctx context.Context, p ConnParams) ([]Setting, error) {
	return m.settings(ctx, p, "SHOW GLOBAL VARIABLES")
}

func (m *MariaDB) settings(ctx context.Context, p ConnParams, q string) ([]Setting, error) {
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Setting{}
	for rows.Next() {
		var s Setting
		var v sql.NullString
		if err := rows.Scan(&s.Name, &v); err != nil {
			return nil, err
		}
		s.Value = v.String
		out = append(out, s)
	}
	return out, rows.Err()
}

// DatabaseStats sums data+index size per schema and counts sessions per
// schema.
func (m *MariaDB) DatabaseStats(ctx context.Context, p ConnParams) ([]DatabaseStat, error) {
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT s.SCHEMA_NAME,
		       COALESCE((SELECT SUM(t.DATA_LENGTH + t.INDEX_LENGTH) FROM information_schema.TABLES t
		                 WHERE t.TABLE_SCHEMA = s.SCHEMA_NAME), 0),
		       (SELECT COUNT(*) FROM information_schema.PROCESSLIST pl WHERE pl.DB = s.SCHEMA_NAME)
		FROM information_schema.SCHEMATA s
		WHERE s.SCHEMA_NAME NOT IN ('information_schema','performance_schema')`)
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
