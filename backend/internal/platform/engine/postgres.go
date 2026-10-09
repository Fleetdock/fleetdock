package engine

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() { Register("postgres", &Postgres{}) }

// Postgres implements Client over the PostgreSQL wire protocol using pgx.
type Postgres struct{}

// connString builds a libpq URL for the given database (empty = "postgres",
// the maintenance database used for create/drop/list).
func (pg *Postgres) connString(p ConnParams, database string) string {
	if database == "" {
		database = "postgres"
	}
	host := p.Host
	if host == "" {
		host = "127.0.0.1"
	}
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(p.User, p.Password),
		Host:   fmt.Sprintf("%s:%d", host, p.Port),
		Path:   "/" + database,
	}
	q := url.Values{}
	mode := p.TLSMode
	if mode == "" {
		mode = TLSPrefer
	}
	q.Set("sslmode", mode)
	q.Set("connect_timeout", "8")
	u.RawQuery = q.Encode()
	return u.String()
}

func (pg *Postgres) connect(ctx context.Context, p ConnParams, database string) (*pgx.Conn, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cfg, err := pgx.ParseConfig(pg.connString(p, database))
	if err != nil {
		return nil, err
	}
	cfg.DialFunc = p.dialFunc()
	if p.SSH != nil {
		// The host is resolved on the bastion: it is often only meaningful
		// there (127.0.0.1, a private DNS name).
		cfg.LookupFunc = func(_ context.Context, host string) ([]string, error) { return []string{host}, nil }
	}
	if t := p.StatementTimeout; t > 0 {
		ms := strconv.FormatInt(t.Milliseconds(), 10)
		cfg.RuntimeParams["statement_timeout"] = ms
		cfg.RuntimeParams["idle_in_transaction_session_timeout"] = ms
	}
	conn, err := pgx.ConnectConfig(cctx, cfg)
	if err != nil || !p.AssumeOwner {
		return conn, err
	}
	var owner *string
	if err := conn.QueryRow(cctx, `
		SELECT r.rolname FROM pg_database d JOIN pg_roles r ON r.oid = d.datdba
		WHERE d.datname = current_database() AND r.rolname <> current_user
		  AND pg_has_role(current_user, r.oid, 'MEMBER')`).Scan(&owner); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		_ = conn.Close(ctx)
		return nil, err
	}
	if owner != nil {
		if _, err := conn.Exec(cctx, "SET ROLE "+quotePGIdent(*owner)); err != nil {
			_ = conn.Close(ctx)
			return nil, err
		}
	}
	return conn, nil
}

// Ping verifies connectivity and returns the server version.
func (pg *Postgres) Ping(ctx context.Context, p ConnParams) (string, error) {
	conn, err := pg.connect(ctx, p, p.Database)
	if err != nil {
		return "", err
	}
	defer conn.Close(ctx)
	var version string
	if err := conn.QueryRow(ctx, "SHOW server_version").Scan(&version); err != nil {
		return "", err
	}
	return "PostgreSQL " + version, nil
}

// ListDatabases returns non-template databases. The maintenance database
// ("postgres") is included but flagged System so it can be browsed and backed
// up without becoming droppable — the control plane connects to it for every
// create/drop/list operation.
func (pg *Postgres) ListDatabases(ctx context.Context, p ConnParams) ([]DatabaseInfo, error) {
	conn, err := pg.connect(ctx, p, "postgres")
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `
		SELECT datname, pg_encoding_to_char(encoding), datcollate
		FROM pg_database
		WHERE datistemplate = false
		ORDER BY datname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DatabaseInfo
	for rows.Next() {
		var d DatabaseInfo
		if err := rows.Scan(&d.Name, &d.Charset, &d.Collation); err != nil {
			return nil, err
		}
		d.System = systemDatabase("postgres", d.Name)
		out = append(out, d)
	}
	return out, rows.Err()
}

// CreateDatabase creates a database if it does not already exist. PostgreSQL
// has no CREATE DATABASE IF NOT EXISTS, so existence is checked first.
func (pg *Postgres) CreateDatabase(ctx context.Context, p ConnParams, name, charset, collation string) error {
	if !identRe.MatchString(name) {
		return fmt.Errorf("invalid database name %q", name)
	}
	conn, err := pg.connect(ctx, p, "postgres")
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = conn.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %s`, quoteIdent(name)))
	return err
}

// DropDatabase drops a database by validated identifier.
func (pg *Postgres) DropDatabase(ctx context.Context, p ConnParams, name string) error {
	if !identRe.MatchString(name) {
		return fmt.Errorf("invalid database name %q", name)
	}
	if systemDatabase("postgres", name) {
		return fmt.Errorf("refusing to drop system database %q", name)
	}
	conn, err := pg.connect(ctx, p, "postgres")
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, quoteIdent(name)))
	return err
}

// CountTables returns the number of user tables in a database.
func (pg *Postgres) CountTables(ctx context.Context, p ConnParams, database string) (int, error) {
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return 0, err
	}
	defer conn.Close(ctx)
	var n int
	err = conn.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema NOT IN ('pg_catalog','information_schema')`).Scan(&n)
	return n, err
}

// DumpArgs builds the pg_dump argv (plain SQL to stdout) and PGPASSWORD env.
func (pg *Postgres) DumpArgs(p ConnParams, database string) ([]string, []string, []string) {
	return []string{"pg_dump"}, []string{
		"--host=" + p.Host,
		"--port=" + strconv.Itoa(p.Port),
		"--username=" + p.User,
		"--no-password",
		"--no-owner",
		"--no-privileges",
		"--clean",
		"--if-exists",
		database,
	}, []string{"PGPASSWORD=" + p.Password}
}

// RestoreArgs builds the psql argv that applies a SQL stream from stdin.
func (pg *Postgres) RestoreArgs(p ConnParams, database string) ([]string, []string, []string) {
	return []string{"psql"}, []string{
		"--host=" + p.Host,
		"--port=" + strconv.Itoa(p.Port),
		"--username=" + p.User,
		"--no-password",
		// Abort on the first error and apply everything in one transaction,
		// so a partly failed restore never reports success.
		"--set=ON_ERROR_STOP=1",
		"--single-transaction",
		"--dbname=" + database,
	}, []string{"PGPASSWORD=" + p.Password}
}

// quoteIdent double-quotes a validated identifier for use in DDL.
func quoteIdent(name string) string { return `"` + name + `"` }
