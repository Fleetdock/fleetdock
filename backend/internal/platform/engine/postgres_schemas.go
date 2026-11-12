package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var _ SchemaAdmin = (*Postgres)(nil)

// schemaExists reports whether a schema of that name exists.
func schemaExists(ctx context.Context, conn *pgx.Conn, name string) (bool, error) {
	var ok bool
	err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`, name).Scan(&ok)
	return ok, err
}

// roleExists reports whether a role of that name exists.
func roleExists(ctx context.Context, conn *pgx.Conn, name string) (bool, error) {
	var ok bool
	err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, name).Scan(&ok)
	return ok, err
}

// checkOwner validates a role name and that the role exists.
func checkOwner(ctx context.Context, conn *pgx.Conn, owner string) error {
	if err := existingName("owner", owner); err != nil {
		return err
	}
	ok, err := roleExists(ctx, conn, owner)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("role %q does not exist", owner)
	}
	return nil
}

// CreateSchema creates a schema. Without an owner it belongs to the database's
// owner, like the database's own objects, so whoever may use the database
// through that role (Fleetdock's read-write data role among them) can use it.
func (pg *Postgres) CreateSchema(ctx context.Context, p ConnParams, database, name, owner string) error {
	if err := validName("schema", name); err != nil {
		return err
	}
	if systemSchema(name) {
		return fmt.Errorf("schema names starting with pg_ are reserved")
	}
	return pg.ddlTx(ctx, p, database, func(ctx context.Context, conn *pgx.Conn) ([]string, error) {
		if ok, err := schemaExists(ctx, conn, name); err != nil {
			return nil, err
		} else if ok {
			return nil, fmt.Errorf("schema %s already exists", name)
		}
		if owner == "" {
			if err := conn.QueryRow(ctx,
				`SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = current_database()`).Scan(&owner); err != nil {
				return nil, err
			}
		} else if err := checkOwner(ctx, conn, owner); err != nil {
			return nil, err
		}
		return []string{"CREATE SCHEMA " + quotePGIdent(name) + " AUTHORIZATION " + quotePGIdent(owner)}, nil
	})
}

// AlterSchema applies a rename, owner and comment change in one transaction.
// The owner and comment apply to the schema under its new name.
func (pg *Postgres) AlterSchema(ctx context.Context, p ConnParams, database, name string, change SchemaChange) error {
	if err := checkUserSchema(name); err != nil {
		return err
	}
	if change.Name == nil && change.Owner == nil && change.Comment == nil {
		return fmt.Errorf("nothing to change")
	}
	return pg.ddlTx(ctx, p, database, func(ctx context.Context, conn *pgx.Conn) ([]string, error) {
		if ok, err := schemaExists(ctx, conn, name); err != nil {
			return nil, err
		} else if !ok {
			return nil, fmt.Errorf("schema %s not found", name)
		}
		var stmts []string
		current := name
		if change.Name != nil && *change.Name != name {
			if err := validName("schema", *change.Name); err != nil {
				return nil, err
			}
			if systemSchema(*change.Name) {
				return nil, fmt.Errorf("schema names starting with pg_ are reserved")
			}
			if ok, err := schemaExists(ctx, conn, *change.Name); err != nil {
				return nil, err
			} else if ok {
				return nil, fmt.Errorf("schema %s already exists", *change.Name)
			}
			stmts = append(stmts, "ALTER SCHEMA "+quotePGIdent(name)+" RENAME TO "+quotePGIdent(*change.Name))
			current = *change.Name
		}
		if change.Owner != nil {
			if err := checkOwner(ctx, conn, *change.Owner); err != nil {
				return nil, err
			}
			stmts = append(stmts, "ALTER SCHEMA "+quotePGIdent(current)+" OWNER TO "+quotePGIdent(*change.Owner))
		}
		if change.Comment != nil {
			comment := "NULL"
			if *change.Comment != "" {
				if len(*change.Comment) > 1024 {
					return nil, fmt.Errorf("a comment is at most 1024 characters")
				}
				comment = pgLiteral(*change.Comment)
			}
			stmts = append(stmts, "COMMENT ON SCHEMA "+quotePGIdent(current)+" IS "+comment)
		}
		return stmts, nil
	})
}

// maxExternalDependents caps the dependents listed from other schemas.
const maxExternalDependents = 50

// SchemaDependents counts the objects in a schema and lists the views and
// foreign keys of other schemas that use it.
func (pg *Postgres) SchemaDependents(ctx context.Context, p ConnParams, database, name string) (*SchemaDependents, error) {
	if err := checkUserSchema(name); err != nil {
		return nil, err
	}
	if !validDBName(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	var nsp uint32
	if err := conn.QueryRow(ctx, `SELECT oid FROM pg_namespace WHERE nspname = $1`, name).Scan(&nsp); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("schema %s not found", name)
		}
		return nil, err
	}
	d := &SchemaDependents{External: []string{}}
	// Objects that belong to an extension go with the extension, not the
	// schema, and are not counted.
	if err := conn.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM pg_class c WHERE c.relnamespace = $1 AND c.relkind IN ('r','p','f')
		     AND NOT EXISTS (SELECT 1 FROM pg_depend e WHERE e.classid = 'pg_class'::regclass AND e.objid = c.oid AND e.deptype = 'e')),
		  (SELECT count(*) FROM pg_class c WHERE c.relnamespace = $1 AND c.relkind IN ('v','m')
		     AND NOT EXISTS (SELECT 1 FROM pg_depend e WHERE e.classid = 'pg_class'::regclass AND e.objid = c.oid AND e.deptype = 'e')),
		  (SELECT count(*) FROM pg_class c WHERE c.relnamespace = $1 AND c.relkind = 'S'
		     AND NOT EXISTS (SELECT 1 FROM pg_depend e WHERE e.classid = 'pg_class'::regclass AND e.objid = c.oid AND e.deptype = 'e')),
		  (SELECT count(*) FROM pg_proc f WHERE f.pronamespace = $1
		     AND NOT EXISTS (SELECT 1 FROM pg_depend e WHERE e.classid = 'pg_proc'::regclass AND e.objid = f.oid AND e.deptype = 'e')),
		  (SELECT count(*) FROM pg_type t WHERE t.typnamespace = $1 AND t.typtype IN ('d','e','r','m')
		     AND NOT EXISTS (SELECT 1 FROM pg_depend e WHERE e.classid = 'pg_type'::regclass AND e.objid = t.oid AND e.deptype = 'e'))`,
		nsp).Scan(&d.Tables, &d.Views, &d.Sequences, &d.Functions, &d.Types); err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, `
		SELECT DISTINCT label FROM (
		  SELECT format('view %I.%I', vn.nspname, v.relname) AS label
		  FROM pg_depend dep
		  JOIN pg_rewrite rw ON dep.classid = 'pg_rewrite'::regclass AND dep.objid = rw.oid
		  JOIN pg_class v ON v.oid = rw.ev_class
		  JOIN pg_namespace vn ON vn.oid = v.relnamespace
		  JOIN pg_class t ON dep.refclassid = 'pg_class'::regclass AND dep.refobjid = t.oid
		  WHERE t.relnamespace = $1 AND v.relnamespace <> $1
		  UNION
		  SELECT format('foreign key %I on %I.%I', con.conname, cn.nspname, c.relname)
		  FROM pg_constraint con
		  JOIN pg_class c ON c.oid = con.conrelid
		  JOIN pg_namespace cn ON cn.oid = c.relnamespace
		  JOIN pg_class r ON r.oid = con.confrelid
		  WHERE con.contype = 'f' AND r.relnamespace = $1 AND c.relnamespace <> $1
		) x ORDER BY label LIMIT $2`, nsp, maxExternalDependents)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		d.External = append(d.External, label)
	}
	return d, rows.Err()
}

// DropSchema drops a schema. Without cascade PostgreSQL refuses while the
// schema holds anything.
func (pg *Postgres) DropSchema(ctx context.Context, p ConnParams, database, name string, cascade bool) error {
	if err := checkUserSchema(name); err != nil {
		return err
	}
	return pg.ddlTx(ctx, p, database, func(ctx context.Context, conn *pgx.Conn) ([]string, error) {
		if ok, err := schemaExists(ctx, conn, name); err != nil {
			return nil, err
		} else if !ok {
			return nil, fmt.Errorf("schema %s not found", name)
		}
		mode := " RESTRICT"
		if cascade {
			mode = " CASCADE"
		}
		return []string{"DROP SCHEMA " + quotePGIdent(name) + mode}, nil
	})
}
