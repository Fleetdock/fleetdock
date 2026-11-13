package engine

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ Extensions = (*Postgres)(nil)

// extensionVersionRe is what extension versions look like ("1.4", "3.4.2",
// "1.0-beta"); anything else is refused before it reaches SQL.
var extensionVersionRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,40}$`)

// Extensions lists pg_available_extensions with what is installed.
func (pg *Postgres) Extensions(ctx context.Context, p ConnParams, database string) ([]Extension, error) {
	out := []Extension{}
	err := pg.withConn(ctx, p, database, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT a.name, coalesce(a.default_version, ''), coalesce(a.comment, ''),
			       coalesce(e.extversion, ''), coalesce(n.nspname, ''),
			       coalesce(v.relocatable, false), coalesce(v.trusted, false), coalesce(v.superuser, true),
			       coalesce((SELECT array_agg(u.target ORDER BY u.target) FROM pg_extension_update_paths(a.name) u
			                  WHERE e.extversion IS NOT NULL AND u.source = e.extversion AND u.path IS NOT NULL), '{}')
			FROM pg_available_extensions a
			LEFT JOIN pg_extension e ON e.extname = a.name
			LEFT JOIN pg_namespace n ON n.oid = e.extnamespace
			LEFT JOIN LATERAL (SELECT relocatable, trusted, superuser FROM pg_available_extension_versions x
			                    WHERE x.name = a.name AND x.version = coalesce(e.extversion, a.default_version) LIMIT 1) v ON true
			ORDER BY e.extname IS NULL, a.name`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var x Extension
			if err := rows.Scan(&x.Name, &x.DefaultVersion, &x.Comment, &x.InstalledVersion, &x.Schema,
				&x.Relocatable, &x.Trusted, &x.Superuser, &x.Updates); err != nil {
				return err
			}
			out = append(out, x)
		}
		return rows.Err()
	})
	return out, err
}

// checkExtension validates a name and that the server offers it.
func checkExtension(ctx context.Context, conn *pgx.Conn, name string) error {
	if err := existingName("extension", name); err != nil {
		return err
	}
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = $1)`, name).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("the server does not offer the extension %s: its files are not installed there", name)
	}
	return nil
}

// extensionError explains the usual refusal: most extensions need a
// superuser, and managed clouds only allow some.
func extensionError(name string, err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "42501" {
		return fmt.Errorf("permission denied for extension %s: it needs a superuser unless it is a trusted extension "+
			"(PostgreSQL 13+); managed services such as RDS or Cloud SQL allow only the extensions on their list: %s", name, pe.Message)
	}
	return err
}

// InstallExtension runs CREATE EXTENSION.
func (pg *Postgres) InstallExtension(ctx context.Context, p ConnParams, database, name, schema, version string) error {
	if version != "" && !extensionVersionRe.MatchString(version) {
		return fmt.Errorf("invalid extension version %q", version)
	}
	return pg.withConn(ctx, p, database, func(conn *pgx.Conn) error {
		if err := checkExtension(ctx, conn, name); err != nil {
			return err
		}
		stmt := "CREATE EXTENSION " + quotePGIdent(name)
		if schema != "" {
			if err := checkUserSchema(schema); err != nil {
				return err
			}
			if ok, err := schemaExists(ctx, conn, schema); err != nil {
				return err
			} else if !ok {
				return fmt.Errorf("schema %s does not exist", schema)
			}
			stmt += " SCHEMA " + quotePGIdent(schema)
		}
		if version != "" {
			stmt += " VERSION " + pgLiteral(version)
		}
		_, err := conn.Exec(ctx, stmt)
		return extensionError(name, err)
	})
}

// UpdateExtension runs ALTER EXTENSION … UPDATE.
func (pg *Postgres) UpdateExtension(ctx context.Context, p ConnParams, database, name, version string) error {
	if version != "" && !extensionVersionRe.MatchString(version) {
		return fmt.Errorf("invalid extension version %q", version)
	}
	return pg.withConn(ctx, p, database, func(conn *pgx.Conn) error {
		if err := existingName("extension", name); err != nil {
			return err
		}
		stmt := "ALTER EXTENSION " + quotePGIdent(name) + " UPDATE"
		if version != "" {
			stmt += " TO " + pgLiteral(version)
		}
		_, err := conn.Exec(ctx, stmt)
		return extensionError(name, err)
	})
}

// DropExtension runs DROP EXTENSION.
func (pg *Postgres) DropExtension(ctx context.Context, p ConnParams, database, name string, cascade bool) error {
	return pg.withConn(ctx, p, database, func(conn *pgx.Conn) error {
		if err := existingName("extension", name); err != nil {
			return err
		}
		if strings.EqualFold(name, "plpgsql") {
			return fmt.Errorf("plpgsql is PostgreSQL's own procedural language and is not dropped from here")
		}
		stmt := "DROP EXTENSION " + quotePGIdent(name)
		if cascade {
			stmt += " CASCADE"
		} else {
			stmt += " RESTRICT"
		}
		_, err := conn.Exec(ctx, stmt)
		return extensionError(name, err)
	})
}
