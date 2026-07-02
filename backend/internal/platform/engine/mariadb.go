package engine

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"time"

	_ "github.com/go-sql-driver/mysql" // driver registration
)

func init() { Register("mariadb", &MariaDB{}) }

// MariaDB implements Client over the MySQL wire protocol.
type MariaDB struct{}

var identRe = regexp.MustCompile(`^[A-Za-z0-9_$]+$`)

func (m *MariaDB) dsn(p ConnParams) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=8s&readTimeout=30s&writeTimeout=30s",
		p.User, p.Password, p.Host, p.Port, p.Database)
}

func (m *MariaDB) open(p ConnParams) (*sql.DB, error) {
	db, err := sql.Open("mysql", m.dsn(p))
	if err != nil {
		return nil, err
	}
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

// ListDatabases returns non-system databases with charset/collation.
func (m *MariaDB) ListDatabases(ctx context.Context, p ConnParams) ([]DatabaseInfo, error) {
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT SCHEMA_NAME, DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME
		FROM information_schema.SCHEMATA
		WHERE SCHEMA_NAME NOT IN ('information_schema','performance_schema','mysql','sys')
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
	db, err := m.open(p)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", name))
	return err
}

// DumpArgs builds the argv for a consistent logical dump to stdout.
func (m *MariaDB) DumpArgs(p ConnParams, database string) ([]string, []string) {
	return []string{"mariadb-dump", "mysqldump"}, []string{
		"--host=" + p.Host,
		fmt.Sprintf("--port=%d", p.Port),
		"--user=" + p.User,
		"--password=" + p.Password,
		"--single-transaction",
		"--quick",
		"--routines",
		"--triggers",
		"--events",
		database,
	}
}

// RestoreArgs builds the argv for restoring a SQL stream from stdin.
func (m *MariaDB) RestoreArgs(p ConnParams, database string) ([]string, []string) {
	return []string{"mariadb", "mysql"}, []string{
		"--host=" + p.Host,
		fmt.Sprintf("--port=%d", p.Port),
		"--user=" + p.User,
		"--password=" + p.Password,
		database,
	}
}
