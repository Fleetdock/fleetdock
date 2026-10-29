package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/schemadiff"
)

var _ SchemaReader = (*Postgres)(nil)

// pgNotExtension filters out objects that belong to an extension (they are
// recreated by CREATE EXTENSION, not by the schema script).
func pgNotExtension(catalog, oidCol string) string {
	return fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM pg_depend dep WHERE dep.classid = '%s'::regclass
		AND dep.objid = %s AND dep.deptype = 'e')`, catalog, oidCol)
}

// ReadSchema reads every user schema's tables, columns, constraints,
// indexes, views, functions, triggers, enum types and extensions with the
// server's own pg_get_*def functions. search_path is emptied first so every
// name in a definition comes out schema-qualified, the same whichever role
// reads it. Partitions are left out (their parent is read).
func (pg *Postgres) ReadSchema(ctx context.Context, p ConnParams, database string) (*schemadiff.Schema, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "SET search_path = pg_catalog"); err != nil {
		return nil, err
	}
	out := &schemadiff.Schema{Dialect: schemadiff.Postgres, Engine: "postgres", Tables: []schemadiff.Table{}}

	rows, err := conn.Query(ctx, `
		SELECT c.oid, n.nspname, c.relname, coalesce(CASE WHEN c.relkind = 'p' THEN 'PARTITION BY ' || pg_get_partkeydef(c.oid) END, '')
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('r', 'p') AND NOT c.relispartition AND `+pgUserSchema("n.nspname")+`
		  AND `+pgNotExtension("pg_class", "c.oid")+`
		ORDER BY n.nspname, c.relname
		LIMIT `+fmt.Sprint(maxSchemaTables+1))
	if err != nil {
		return nil, err
	}
	byOid := map[uint32]int{}
	var oids []uint32
	for rows.Next() {
		var oid uint32
		var t schemadiff.Table
		if err := rows.Scan(&oid, &t.Schema, &t.Name, &t.Options); err != nil {
			rows.Close()
			return nil, err
		}
		if len(out.Tables) == maxSchemaTables {
			out.Truncated = true
			continue
		}
		t.Columns = []schemadiff.Column{}
		byOid[oid] = len(out.Tables)
		oids = append(oids, oid)
		out.Tables = append(out.Tables, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	err = pgEach(ctx, conn, `
		SELECT a.attrelid, a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull,
		       coalesce(pg_get_expr(ad.adbin, ad.adrelid), ''), a.attidentity::text, a.attgenerated::text,
		       coalesce(CASE WHEN a.attcollation <> 0 AND a.attcollation <> t.typcollation THEN
		         (SELECT quote_ident(cn.nspname) || '.' || quote_ident(co.collname) FROM pg_collation co
		          JOIN pg_namespace cn ON cn.oid = co.collnamespace WHERE co.oid = a.attcollation) END, '')
		FROM pg_attribute a
		JOIN pg_type t ON t.oid = a.atttypid
		LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
		WHERE a.attrelid = ANY($1::oid[]) AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attrelid, a.attnum`, []any{oids}, func(r pgx.Rows) error {
		var oid uint32
		var c schemadiff.Column
		var expr, identity, generated string
		if err := r.Scan(&oid, &c.Name, &c.Type, &c.NotNull, &expr, &identity, &generated, &c.Collation); err != nil {
			return err
		}
		if generated == "s" {
			c.Generated = expr
		} else {
			c.Default = expr
		}
		switch identity {
		case "a":
			c.Identity = "ALWAYS"
		case "d":
			c.Identity = "BY DEFAULT"
		}
		c.Def = schemadiff.PGColumnDef(c)
		t := &out.Tables[byOid[oid]]
		t.Columns = append(t.Columns, c)
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = pgEach(ctx, conn, `
		SELECT conrelid, conname, contype::text, pg_get_constraintdef(oid)
		FROM pg_constraint WHERE conrelid = ANY($1::oid[]) AND contype IN ('p', 'u', 'c', 'f', 'x')`,
		[]any{oids}, func(r pgx.Rows) error {
			var oid uint32
			var o schemadiff.Object
			if err := r.Scan(&oid, &o.Name, &o.Kind, &o.Def); err != nil {
				return err
			}
			t := &out.Tables[byOid[oid]]
			t.Constraints = append(t.Constraints, o)
			return nil
		})
	if err != nil {
		return nil, err
	}

	err = pgEach(ctx, conn, `
		SELECT i.indrelid, ic.relname, pg_get_indexdef(i.indexrelid)
		FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid
		WHERE i.indrelid = ANY($1::oid[])
		  AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conindid = i.indexrelid
		                  AND c.conrelid = i.indrelid AND c.contype IN ('p', 'u', 'x'))`,
		[]any{oids}, func(r pgx.Rows) error {
			var oid uint32
			var o schemadiff.Object
			if err := r.Scan(&oid, &o.Name, &o.Def); err != nil {
				return err
			}
			o.Kind = "INDEX"
			t := &out.Tables[byOid[oid]]
			t.Indexes = append(t.Indexes, o)
			return nil
		})
	if err != nil {
		return nil, err
	}

	err = pgEach(ctx, conn, `
		SELECT n.nspname, c.relname, c.relkind::text, pg_get_viewdef(c.oid, true)
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('v', 'm') AND `+pgUserSchema("n.nspname")+` AND `+pgNotExtension("pg_class", "c.oid"),
		nil, func(r pgx.Rows) error {
			var o schemadiff.Object
			var kind, def string
			if err := r.Scan(&o.Schema, &o.Name, &kind, &def); err != nil {
				return err
			}
			def = strings.TrimSuffix(strings.TrimSpace(def), ";")
			name := schemadiff.QuotePG(o.Schema) + "." + schemadiff.QuotePG(o.Name)
			if kind == "m" {
				o.Kind, o.Def = "MATERIALIZED", "CREATE MATERIALIZED VIEW "+name+" AS\n"+def
			} else {
				o.Kind, o.Def = "VIEW", "CREATE OR REPLACE VIEW "+name+" AS\n"+def
			}
			out.Views = append(out.Views, o)
			return nil
		})
	if err != nil {
		return nil, err
	}

	err = pgEach(ctx, conn, `
		SELECT n.nspname, p.proname, pg_get_function_identity_arguments(p.oid), p.prokind::text, pg_get_functiondef(p.oid)
		FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE p.prokind IN ('f', 'p') AND `+pgUserSchema("n.nspname")+` AND `+pgNotExtension("pg_proc", "p.oid")+`
		ORDER BY 1, 2, 3
		LIMIT `+fmt.Sprint(maxSchemaRoutines+1), nil, func(r pgx.Rows) error {
		var o schemadiff.Object
		var name, args, kind string
		if err := r.Scan(&o.Schema, &name, &args, &kind, &o.Def); err != nil {
			return err
		}
		if len(out.Routines) == maxSchemaRoutines {
			out.Truncated = true
			return nil
		}
		o.Name = name + "(" + args + ")"
		o.Kind = "FUNCTION"
		if kind == "p" {
			o.Kind = "PROCEDURE"
		}
		o.Def = strings.TrimSpace(o.Def)
		o.Drop = fmt.Sprintf("DROP %s %s.%s(%s)", o.Kind, schemadiff.QuotePG(o.Schema), schemadiff.QuotePG(name), args)
		out.Routines = append(out.Routines, o)
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = pgEach(ctx, conn, `
		SELECT n.nspname, c.relname, t.tgname, pg_get_triggerdef(t.oid)
		FROM pg_trigger t
		JOIN pg_class c ON c.oid = t.tgrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE NOT t.tgisinternal AND NOT c.relispartition AND `+pgUserSchema("n.nspname"),
		nil, func(r pgx.Rows) error {
			var o schemadiff.Object
			if err := r.Scan(&o.Schema, &o.Table, &o.Name, &o.Def); err != nil {
				return err
			}
			o.Kind = "TRIGGER"
			out.Triggers = append(out.Triggers, o)
			return nil
		})
	if err != nil {
		return nil, err
	}

	err = pgEach(ctx, conn, `
		SELECT n.nspname, t.typname, array_agg(e.enumlabel::text ORDER BY e.enumsortorder)
		FROM pg_type t
		JOIN pg_enum e ON e.enumtypid = t.oid
		JOIN pg_namespace n ON n.oid = t.typnamespace
		WHERE `+pgUserSchema("n.nspname")+` AND `+pgNotExtension("pg_type", "t.oid")+`
		GROUP BY 1, 2`, nil, func(r pgx.Rows) error {
		var o schemadiff.Object
		if err := r.Scan(&o.Schema, &o.Name, &o.Values); err != nil {
			return err
		}
		labels := make([]string, len(o.Values))
		for i, v := range o.Values {
			labels[i] = schemadiff.PGLiteral(v)
		}
		o.Kind = "ENUM"
		o.Def = fmt.Sprintf("CREATE TYPE %s.%s AS ENUM (%s)", schemadiff.QuotePG(o.Schema), schemadiff.QuotePG(o.Name), strings.Join(labels, ", "))
		out.Types = append(out.Types, o)
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = pgEach(ctx, conn, `
		SELECT e.extname, n.nspname FROM pg_extension e JOIN pg_namespace n ON n.oid = e.extnamespace
		WHERE e.extname <> 'plpgsql'`, nil, func(r pgx.Rows) error {
		var name, schema string
		if err := r.Scan(&name, &schema); err != nil {
			return err
		}
		out.Extensions = append(out.Extensions, schemadiff.Object{
			Name: name, Kind: "EXTENSION",
			Def: fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s WITH SCHEMA %s", schemadiff.QuotePG(name), schemadiff.QuotePG(schema)),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	out.Canonicalize()
	return out, nil
}

// pgEach runs a query and calls fn for every row.
func pgEach(ctx context.Context, conn *pgx.Conn, q string, args []any, fn func(pgx.Rows) error) error {
	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
