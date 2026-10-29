package engine

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/schemadiff"
)

var _ SchemaReader = (*MariaDB)(nil)

var (
	autoIncrementOpt = regexp.MustCompile(`\s*AUTO_INCREMENT=\d+`)
	definerClause    = regexp.MustCompile(`\s+DEFINER=\S+`)
)

// ReadSchema reads tables with SHOW CREATE TABLE (whose column and index
// lines are exactly what ALTER TABLE accepts), views, routines and triggers.
// DEFINER clauses, AUTO_INCREMENT counters and the database name qualifying
// view bodies are dropped, so the same structure in two databases compares
// equal.
func (m *MariaDB) ReadSchema(ctx context.Context, p ConnParams, database string) (*schemadiff.Schema, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	out := &schemadiff.Schema{Dialect: schemadiff.MySQL, Tables: []schemadiff.Table{}}
	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return nil, err
	}
	out.Engine = "mysql"
	if strings.Contains(strings.ToLower(version), "mariadb") {
		out.Engine = "mariadb"
	}
	qdb := schemadiff.QuoteMySQL(database)

	tables, err := stringColumn(ctx, db, `
		SELECT TABLE_NAME FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME`, database)
	if err != nil {
		return nil, err
	}
	if len(tables) > maxSchemaTables {
		tables, out.Truncated = tables[:maxSchemaTables], true
	}
	for _, name := range tables {
		var tn, create string
		if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE "+qdb+"."+schemadiff.QuoteMySQL(name)).Scan(&tn, &create); err != nil {
			return nil, fmt.Errorf("SHOW CREATE TABLE %s: %w", name, err)
		}
		out.Tables = append(out.Tables, ParseMySQLCreateTable(name, create))
	}

	vrows, err := db.QueryContext(ctx, `
		SELECT TABLE_NAME, VIEW_DEFINITION, CHECK_OPTION, SECURITY_TYPE
		FROM information_schema.VIEWS WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME`, database)
	if err != nil {
		return nil, err
	}
	for vrows.Next() {
		var name, def, check, security string
		if err := vrows.Scan(&name, &def, &check, &security); err != nil {
			_ = vrows.Close()
			return nil, err
		}
		def = strings.ReplaceAll(def, qdb+".", "")
		s := fmt.Sprintf("CREATE OR REPLACE SQL SECURITY %s VIEW %s AS %s", security, schemadiff.QuoteMySQL(name), def)
		if check != "" && check != "NONE" {
			s += " WITH " + check + " CHECK OPTION"
		}
		out.Views = append(out.Views, schemadiff.Object{Name: name, Kind: "VIEW", Def: s})
	}
	_ = vrows.Close()
	if err := vrows.Err(); err != nil {
		return nil, err
	}

	rrows, err := db.QueryContext(ctx, `
		SELECT ROUTINE_NAME, ROUTINE_TYPE FROM information_schema.ROUTINES
		WHERE ROUTINE_SCHEMA = ? ORDER BY ROUTINE_NAME, ROUTINE_TYPE`, database)
	if err != nil {
		return nil, err
	}
	type routine struct{ name, kind string }
	var routines []routine
	for rrows.Next() {
		var r routine
		if err := rrows.Scan(&r.name, &r.kind); err != nil {
			_ = rrows.Close()
			return nil, err
		}
		routines = append(routines, r)
	}
	_ = rrows.Close()
	if err := rrows.Err(); err != nil {
		return nil, err
	}
	if len(routines) > maxSchemaRoutines {
		routines, out.Truncated = routines[:maxSchemaRoutines], true
	}
	for _, r := range routines {
		def, err := showCreate(ctx, db, "SHOW CREATE "+r.kind+" "+qdb+"."+schemadiff.QuoteMySQL(r.name), 2)
		if err != nil {
			return nil, err
		}
		if def == "" {
			continue // no privilege to see the body
		}
		out.Routines = append(out.Routines, schemadiff.Object{Name: r.name, Kind: r.kind, Def: definerClause.ReplaceAllString(def, "")})
	}

	trows, err := db.QueryContext(ctx, `
		SELECT TRIGGER_NAME, ACTION_TIMING, EVENT_MANIPULATION, EVENT_OBJECT_TABLE, ACTION_STATEMENT
		FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA = ? ORDER BY TRIGGER_NAME`, database)
	if err != nil {
		return nil, err
	}
	defer trows.Close()
	for trows.Next() {
		var name, timing, event, table, stmt string
		if err := trows.Scan(&name, &timing, &event, &table, &stmt); err != nil {
			return nil, err
		}
		out.Triggers = append(out.Triggers, schemadiff.Object{
			Name: name, Table: table, Kind: "TRIGGER",
			Def: fmt.Sprintf("CREATE TRIGGER %s %s %s ON %s FOR EACH ROW %s", schemadiff.QuoteMySQL(name), timing, event,
				schemadiff.QuoteMySQL(table), stmt),
		})
	}
	if err := trows.Err(); err != nil {
		return nil, err
	}
	out.Canonicalize()
	return out, nil
}

func stringColumn(ctx context.Context, db *sql.DB, q string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// showCreate runs a SHOW CREATE statement and returns column col ("" when
// it is NULL: the login may not see the body).
func showCreate(ctx context.Context, db *sql.DB, q string, col int) (string, error) {
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}
	if !rows.Next() {
		return "", rows.Err()
	}
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return "", err
	}
	if col >= len(vals) {
		return "", nil
	}
	return vals[col].String, nil
}

// ParseMySQLCreateTable splits SHOW CREATE TABLE output into columns,
// indexes, constraints and table options.
func ParseMySQLCreateTable(name, create string) schemadiff.Table {
	t := schemadiff.Table{Name: name, Columns: []schemadiff.Column{}}
	lines := strings.Split(create, "\n")
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, ")") {
			opts := strings.TrimSpace(strings.TrimPrefix(line, ")"))
			if rest := strings.TrimSpace(strings.Join(lines[i+1:], "\n")); rest != "" {
				opts += "\n" + rest
			}
			t.Options = strings.TrimSpace(autoIncrementOpt.ReplaceAllString(opts, ""))
			break
		}
		def := strings.TrimSuffix(line, ",")
		upper := strings.ToUpper(def)
		switch {
		case strings.HasPrefix(def, "`"):
			t.Columns = append(t.Columns, schemadiff.Column{Name: firstIdent(def), Def: def})
		case strings.HasPrefix(upper, "PRIMARY KEY"):
			t.Indexes = append(t.Indexes, schemadiff.Object{Name: "PRIMARY", Kind: "PRIMARY", Def: def})
		case strings.HasPrefix(upper, "UNIQUE"):
			t.Indexes = append(t.Indexes, schemadiff.Object{Name: firstIdent(def), Kind: "UNIQUE", Def: def})
		case strings.HasPrefix(upper, "CONSTRAINT"):
			kind := "CHECK"
			if strings.Contains(upper, " FOREIGN KEY ") {
				kind = "FOREIGN"
			}
			t.Constraints = append(t.Constraints, schemadiff.Object{Name: firstIdent(def), Kind: kind, Def: def})
		case strings.Contains(upper, "KEY ") || strings.Contains(upper, "INDEX "):
			t.Indexes = append(t.Indexes, schemadiff.Object{Name: firstIdent(def), Kind: "INDEX", Def: def})
		default:
			t.Constraints = append(t.Constraints, schemadiff.Object{Name: def, Kind: "OTHER", Def: def})
		}
	}
	return t
}

// firstIdent returns the first `quoted` identifier in s, unquoted.
func firstIdent(s string) string {
	i := strings.IndexByte(s, '`')
	if i < 0 {
		return ""
	}
	j := skipQuoted(s, i, '`')
	return unquoteIdent(s[i:j])
}
