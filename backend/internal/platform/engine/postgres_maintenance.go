package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var _ Maintenance = (*Postgres)(nil)

// relationOf checks that schema.name is a relation of one of the kinds and
// returns its quoted, qualified name.
func relationOf(ctx context.Context, conn *pgx.Conn, schema, name, noun string, kinds ...string) (string, error) {
	if err := existingName("schema", schema); err != nil {
		return "", err
	}
	if err := existingName(noun, name); err != nil {
		return "", err
	}
	var kind string
	err := conn.QueryRow(ctx, `SELECT c.relkind::text FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2`, schema, name).Scan(&kind)
	if err == pgx.ErrNoRows {
		return "", fmt.Errorf("%s %s.%s does not exist", noun, schema, name)
	}
	if err != nil {
		return "", err
	}
	for _, k := range kinds {
		if k == kind {
			return quotePGIdent(schema) + "." + quotePGIdent(name), nil
		}
	}
	return "", fmt.Errorf("%s.%s is not a %s", schema, name, noun)
}

// withConn opens a session on the database.
func (pg *Postgres) withConn(ctx context.Context, p ConnParams, database string, fn func(conn *pgx.Conn) error) error {
	if !validDBName(database) {
		return fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	return fn(conn)
}

// Sequence reads a sequence from pg_sequences and who owns it.
func (pg *Postgres) Sequence(ctx context.Context, p ConnParams, database, schema, name string) (*SequenceInfo, error) {
	var out *SequenceInfo
	err := pg.withConn(ctx, p, database, func(conn *pgx.Conn) error {
		if _, err := relationOf(ctx, conn, schema, name, "sequence", "S"); err != nil {
			return err
		}
		s := SequenceInfo{Schema: schema, Name: name}
		err := conn.QueryRow(ctx, `
			SELECT s.data_type::text, s.start_value, s.increment_by, s.min_value, s.max_value, s.cycle, s.last_value,
			       coalesce((SELECT format('%s.%s.%s', tn.nspname, t.relname, a.attname)
			                   FROM pg_depend d
			                   JOIN pg_class t ON t.oid = d.refobjid
			                   JOIN pg_namespace tn ON tn.oid = t.relnamespace
			                   JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = d.refobjsubid
			                  WHERE d.classid = 'pg_class'::regclass AND d.objid = format('%I.%I', s.schemaname, s.sequencename)::regclass
			                    AND d.refclassid = 'pg_class'::regclass AND d.deptype IN ('a', 'i')
			                  LIMIT 1), '')
			FROM pg_sequences s WHERE s.schemaname = $1 AND s.sequencename = $2`, schema, name).
			Scan(&s.DataType, &s.Start, &s.Increment, &s.Min, &s.Max, &s.Cycle, &s.LastValue, &s.OwnedBy)
		if err != nil {
			return err
		}
		// The sequence itself knows whether its value was handed out yet,
		// which pg_sequences hides behind a NULL.
		var last int64
		var called bool
		q := quotePGIdent(schema) + "." + quotePGIdent(name)
		if err := conn.QueryRow(ctx, "SELECT last_value, is_called FROM "+q).Scan(&last, &called); err != nil {
			return err
		}
		next := last
		if called {
			next = last + s.Increment
			if (s.Increment > 0 && (next > s.Max || next < last)) || (s.Increment < 0 && (next < s.Min || next > last)) {
				if s.Cycle {
					if s.Increment > 0 {
						next = s.Min
					} else {
						next = s.Max
					}
				} else {
					out = &s
					return nil
				}
			}
		}
		s.NextValue = &next
		out = &s
		return nil
	})
	return out, err
}

// SetSequence runs setval.
func (pg *Postgres) SetSequence(ctx context.Context, p ConnParams, database, schema, name string, value int64, called bool) error {
	return pg.withConn(ctx, p, database, func(conn *pgx.Conn) error {
		q, err := relationOf(ctx, conn, schema, name, "sequence", "S")
		if err != nil {
			return err
		}
		_, err = conn.Exec(ctx, `SELECT setval($1::regclass, $2, $3)`, q, value, called)
		return err
	})
}

// RefreshMaterializedView runs REFRESH MATERIALIZED VIEW.
func (pg *Postgres) RefreshMaterializedView(ctx context.Context, p ConnParams, database, schema, name string, concurrently bool) error {
	return pg.withConn(ctx, p, database, func(conn *pgx.Conn) error {
		q, err := relationOf(ctx, conn, schema, name, "materialized view", "m")
		if err != nil {
			return err
		}
		stmt := "REFRESH MATERIALIZED VIEW "
		if concurrently {
			// PostgreSQL's own error does not say what is missing.
			var populated, unique bool
			if err := conn.QueryRow(ctx, `SELECT c.relispopulated,
				EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid = c.oid AND i.indisunique AND i.indpred IS NULL
				          AND NOT EXISTS (SELECT 1 FROM unnest(i.indkey) k WHERE k = 0))
				FROM pg_class c WHERE c.oid = $1::regclass`, q).Scan(&populated, &unique); err != nil {
				return err
			}
			if !unique {
				return fmt.Errorf("%s.%s cannot be refreshed concurrently: it needs a unique index on plain columns, without WHERE", schema, name)
			}
			if !populated {
				return fmt.Errorf("%s.%s has never been filled: refresh it once without CONCURRENTLY", schema, name)
			}
			stmt += "CONCURRENTLY "
		}
		_, err = conn.Exec(ctx, stmt+q)
		return err
	})
}

// Maintain runs a housekeeping command. VACUUM cannot run inside a
// transaction, so each statement runs on its own.
func (pg *Postgres) Maintain(ctx context.Context, p ConnParams, database string, op MaintenanceOp, schema, table string) ([]string, error) {
	if !op.Valid() {
		return nil, fmt.Errorf("unknown maintenance %q: use vacuum, vacuum_analyze, analyze or reindex", op)
	}
	var ran []string
	err := pg.withConn(ctx, p, database, func(conn *pgx.Conn) error {
		var targets []string
		if table != "" {
			q, err := relationOf(ctx, conn, schema, table, "table", "r", "p", "m")
			if err != nil {
				return err
			}
			targets = []string{q}
		} else {
			if err := checkUserSchema(schema); err != nil {
				return err
			}
			if ok, err := schemaExists(ctx, conn, schema); err != nil {
				return err
			} else if !ok {
				return fmt.Errorf("schema %s does not exist", schema)
			}
			if op == OpReindex {
				targets = []string{"SCHEMA " + quotePGIdent(schema)}
			} else {
				rows, err := conn.Query(ctx, `SELECT format('%I.%I', n.nspname, c.relname) FROM pg_class c
					JOIN pg_namespace n ON n.oid = c.relnamespace
					WHERE n.nspname = $1 AND c.relkind IN ('r', 'm') ORDER BY c.relname`, schema)
				if err != nil {
					return err
				}
				for rows.Next() {
					var t string
					if err := rows.Scan(&t); err != nil {
						rows.Close()
						return err
					}
					targets = append(targets, t)
				}
				rows.Close()
				if err := rows.Err(); err != nil {
					return err
				}
				if len(targets) == 0 {
					return fmt.Errorf("schema %s has no tables", schema)
				}
			}
		}
		verb := map[MaintenanceOp]string{OpVacuum: "VACUUM", OpVacuumAnalyze: "VACUUM (ANALYZE)", OpAnalyze: "ANALYZE", OpReindex: "REINDEX"}[op]
		for _, t := range targets {
			stmt := verb + " " + t
			if op == OpReindex && table != "" {
				stmt = "REINDEX TABLE " + t
			}
			if _, err := conn.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("%s: %w", stmt, err)
			}
			ran = append(ran, stmt)
		}
		return nil
	})
	return ran, err
}

// TableStats reads pg_stat_user_tables.
func (pg *Postgres) TableStats(ctx context.Context, p ConnParams, database, schema string) ([]TableStat, error) {
	out := []TableStat{}
	err := pg.withConn(ctx, p, database, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT s.schemaname, s.relname, pg_total_relation_size(s.relid), s.n_live_tup, s.n_dead_tup,
			       s.last_vacuum, s.last_autovacuum, s.last_analyze, s.last_autoanalyze
			FROM pg_stat_user_tables s
			JOIN pg_class c ON c.oid = s.relid
			-- A partitioned table has no storage of its own; its partitions
			-- are listed.
			WHERE ($1 = '' OR s.schemaname = $1) AND c.relkind <> 'p'
			ORDER BY s.schemaname, s.relname`, schema)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t TableStat
			var lv, lav, la, laa *time.Time
			if err := rows.Scan(&t.Schema, &t.Name, &t.TotalBytes, &t.LiveRows, &t.DeadRows, &lv, &lav, &la, &laa); err != nil {
				return err
			}
			t.LastVacuum, t.LastAutovacuum, t.LastAnalyze, t.LastAutoanalyze = lv, lav, la, laa
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

// Partitions lists a partitioned table's direct partitions.
func (pg *Postgres) Partitions(ctx context.Context, p ConnParams, database, schema, table string) ([]Partition, error) {
	out := []Partition{}
	err := pg.withConn(ctx, p, database, func(conn *pgx.Conn) error {
		q, err := relationOf(ctx, conn, schema, table, "partitioned table", "p")
		if err != nil {
			return err
		}
		rows, err := conn.Query(ctx, `
			SELECT n.nspname, c.relname, coalesce(pg_get_expr(c.relpartbound, c.oid), ''),
			       coalesce((SELECT sum(pg_total_relation_size(t.relid)) FROM pg_partition_tree(c.oid) t), 0)::bigint,
			       coalesce((SELECT sum(greatest(k.reltuples, 0)) FROM pg_partition_tree(c.oid) t
			                   JOIN pg_class k ON k.oid = t.relid WHERE t.isleaf), 0)::bigint,
			       c.relkind = 'p'
			FROM pg_inherits i
			JOIN pg_class c ON c.oid = i.inhrelid
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE i.inhparent = $1::regclass
			ORDER BY c.relname`, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pt Partition
			if err := rows.Scan(&pt.Schema, &pt.Name, &pt.Bound, &pt.TotalBytes, &pt.Rows, &pt.Partitioned); err != nil {
				return err
			}
			out = append(out, pt)
		}
		return rows.Err()
	})
	return out, err
}
