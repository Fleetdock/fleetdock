package engine

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/netsafe"
)

// mysqlNet is a custom network name whose dialer enforces the netsafe
// database policy on the address actually connected to.
const mysqlNet = "fleetdock-tcp"

// MariaDB and MySQL share the MySQL wire protocol and client tooling, so one
// implementation serves both engines.
func init() {
	mysql.RegisterDialContext(mysqlNet, func(ctx context.Context, addr string) (net.Conn, error) {
		return netsafe.DialDB(ctx, "tcp", addr)
	})
	Register("mariadb", &MariaDB{})
	Register("mysql", &MariaDB{})
}

// MariaDB implements Client over the MySQL wire protocol.
type MariaDB struct{}

var identRe = regexp.MustCompile(`^[A-Za-z0-9_$]+$`)

// dsn builds the driver DSN. It goes through mysql.Config rather than string
// formatting so passwords containing '@', '/' or '?' cannot corrupt it.
func (m *MariaDB) dsn(p ConnParams) string { return m.config(p).FormatDSN() }

// config builds the driver configuration. With an SSH tunnel the dial goes
// through the bastion instead of the registered netsafe network.
func (m *MariaDB) config(p ConnParams) *mysql.Config {
	c := mysql.NewConfig()
	c.User = p.User
	c.Passwd = p.Password
	c.Net = mysqlNet
	c.Addr = net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	c.DBName = p.Database
	c.Timeout = 8 * time.Second
	c.ReadTimeout = 30 * time.Second
	c.WriteTimeout = 30 * time.Second
	c.TLSConfig = mysqlTLS(p.TLSMode)
	// Report rows matched, not rows changed, for UPDATE: re-saving a row with
	// identical values must count as one row, or single-row edits would be
	// refused as "not found".
	c.ClientFoundRows = true
	if p.SSH != nil {
		c.DialFunc = p.dialFunc()
	}
	return c
}

// mysqlTLS maps a libpq-style TLS mode onto the driver's tls parameter.
func mysqlTLS(mode string) string {
	switch mode {
	case TLSDisable:
		return "false"
	case TLSRequire:
		return "skip-verify"
	case TLSVerifyFull:
		return "true"
	default:
		return "preferred"
	}
}

func (m *MariaDB) open(p ConnParams) (*sql.DB, error) {
	conn, err := mysql.NewConnector(m.config(p))
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(conn)
	db.SetConnMaxLifetime(time.Minute)
	db.SetMaxOpenConns(2)
	return db, nil
}

// Ping verifies connectivity and returns the server version.
func (m *MariaDB) Ping(ctx context.Context, p ConnParams) (string, error) {
	db, err := m.open(p)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return "", err
	}
	return version, nil
}

// ListDatabases returns user databases plus the dumpable system schemas
// (mysql, sys), which are flagged System so they can be browsed and backed up
// without becoming droppable. The virtual schemas information_schema and
// performance_schema are excluded — mysqldump cannot produce a usable dump of
// them.
func (m *MariaDB) ListDatabases(ctx context.Context, p ConnParams) ([]DatabaseInfo, error) {
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT SCHEMA_NAME, DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME
		FROM information_schema.SCHEMATA
		WHERE SCHEMA_NAME NOT IN ('information_schema','performance_schema')
		ORDER BY SCHEMA_NAME`)
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
		d.System = systemDatabase("mysql", d.Name)
		out = append(out, d)
	}
	return out, rows.Err()
}

// CreateDatabase creates a database; identifiers are validated, not escaped.
func (m *MariaDB) CreateDatabase(ctx context.Context, p ConnParams, name, charset, collation string) error {
	if !identRe.MatchString(name) {
		return fmt.Errorf("invalid database name %q", name)
	}
	if charset == "" {
		charset = "utf8mb4"
	}
	if collation == "" {
		collation = "utf8mb4_unicode_ci"
	}
	if !identRe.MatchString(charset) || !identRe.MatchString(collation) {
		return fmt.Errorf("invalid charset/collation")
	}
	db, err := m.open(p)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET %s COLLATE %s", name, charset, collation))
	return err
}

// DropDatabase drops a database by validated identifier.
func (m *MariaDB) DropDatabase(ctx context.Context, p ConnParams, name string) error {
	if !identRe.MatchString(name) {
		return fmt.Errorf("invalid database name %q", name)
	}
	if systemDatabase("mysql", name) {
		return fmt.Errorf("refusing to drop system database %q", name)
	}
	db, err := m.open(p)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", name))
	return err
}

// CountTables returns the number of base tables in a database.
func (m *MariaDB) CountTables(ctx context.Context, p ConnParams, database string) (int, error) {
	p.Database = database
	db, err := m.open(p)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var n int
	err = db.QueryRowContext(ctx,
		"SELECT count(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ?", database).Scan(&n)
	return n, err
}

// DumpArgs builds the argv for a consistent logical dump to stdout.
func (m *MariaDB) DumpArgs(p ConnParams, database string) ([]string, []string, []string) {
	return []string{"mariadb-dump", "mysqldump"}, []string{
		"--host=" + p.Host,
		fmt.Sprintf("--port=%d", p.Port),
		"--user=" + p.User,
		"--single-transaction",
		"--quick",
		"--routines",
		"--triggers",
		"--events",
		database,
	}, []string{"MYSQL_PWD=" + p.Password}
}

// RestoreArgs builds the argv for restoring a SQL stream from stdin.
func (m *MariaDB) RestoreArgs(p ConnParams, database string) ([]string, []string, []string) {
	return []string{"mariadb", "mysql"}, []string{
		"--host=" + p.Host,
		fmt.Sprintf("--port=%d", p.Port),
		"--user=" + p.User,
		database,
	}, []string{"MYSQL_PWD=" + p.Password}
}
