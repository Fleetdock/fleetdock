package engine

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var _ Admin = (*Postgres)(nil)

func validPGRole(name string) bool {
	return identRe.MatchString(name)
}

func quotePGIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (pg *Postgres) tableSchema(ctx context.Context, conn *pgx.Conn, table string) (string, error) {
	var schema string
	err := conn.QueryRow(ctx, `
		SELECT table_schema FROM information_schema.tables
		WHERE table_name = $1
		  AND table_schema NOT IN ('pg_catalog', 'information_schema')
		ORDER BY CASE WHEN table_schema = 'public' THEN 0 ELSE 1 END
		LIMIT 1`, table).Scan(&schema)
	return schema, err
}

func qualifiedTable(schema, table string) string {
	return quotePGIdent(schema) + "." + quotePGIdent(table)
}

// ListDBUsers returns login roles (PostgreSQL has no user@host model).
func (pg *Postgres) ListDBUsers(ctx context.Context, p ConnParams) ([]DBUser, error) {
	conn, err := pg.connect(ctx, p, "postgres")
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT rolname FROM pg_roles
		WHERE rolcanlogin AND rolname NOT LIKE 'pg\_%'
		ORDER BY rolname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DBUser
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, DBUser{User: name, Host: ""})
	}
	return out, rows.Err()
}

// CreateDBUser creates a login role. The host parameter is ignored.
func (pg *Postgres) CreateDBUser(ctx context.Context, p ConnParams, user, host, password string) error {
	if !validPGRole(user) {
		return fmt.Errorf("invalid role name")
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}
	conn, err := pg.connect(ctx, p, "postgres")
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	pw := strings.ReplaceAll(password, "'", "''")
	_, err = conn.Exec(ctx, fmt.Sprintf(
		"CREATE ROLE %s WITH LOGIN PASSWORD '%s'", quotePGIdent(user), pw))
	return err
}

// DropDBUser removes a login role. The host parameter is ignored.
func (pg *Postgres) DropDBUser(ctx context.Context, p ConnParams, user, host string) error {
	if !validPGRole(user) {
		return fmt.Errorf("invalid role name")
	}
	conn, err := pg.connect(ctx, p, "postgres")
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, "DROP ROLE IF EXISTS "+quotePGIdent(user))
	return err
}

// UserGrants returns human-readable grant lines for a role.
func (pg *Postgres) UserGrants(ctx context.Context, p ConnParams, user, host string) ([]string, error) {
	if !validPGRole(user) {
		return nil, fmt.Errorf("invalid role name")
	}
	conn, err := pg.connect(ctx, p, "postgres")
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	var out []string
	dbRows, err := conn.Query(ctx, `
		SELECT p.privilege_type
		FROM (VALUES ('CONNECT'), ('CREATE'), ('TEMPORARY')) AS p(privilege_type)
		WHERE has_database_privilege($1::name, current_database(), p.privilege_type)`,
		user)
	if err == nil {
		defer dbRows.Close()
		var dbPrivs []string
		for dbRows.Next() {
			var priv string
			if err := dbRows.Scan(&priv); err != nil {
				return nil, err
			}
			dbPrivs = append(dbPrivs, priv)
		}
		if err := dbRows.Err(); err != nil {
			return nil, err
		}
		if len(dbPrivs) > 0 {
			var dbName string
			_ = conn.QueryRow(ctx, "SELECT current_database()").Scan(&dbName)
			out = append(out, fmt.Sprintf("GRANT %s ON DATABASE %s TO %s",
				strings.Join(dbPrivs, ", "), quoteIdent(dbName), quotePGIdent(user)))
		}
	}

	schemaRows, err := conn.Query(ctx, `
		SELECT privilege_type FROM information_schema.schema_privileges
		WHERE grantee = $1 AND schema_name = 'public'
		ORDER BY privilege_type`, user)
	if err == nil {
		defer schemaRows.Close()
		var privs []string
		for schemaRows.Next() {
			var priv string
			if err := schemaRows.Scan(&priv); err != nil {
				return nil, err
			}
			privs = append(privs, priv)
		}
		if err := schemaRows.Err(); err != nil {
			return nil, err
		}
		if len(privs) > 0 {
			out = append(out, fmt.Sprintf("GRANT %s ON SCHEMA public TO %s",
				strings.Join(privs, ", "), quotePGIdent(user)))
		}
	}

	tableRows, err := conn.Query(ctx, `
		SELECT table_schema, table_name, privilege_type
		FROM information_schema.table_privileges
		WHERE grantee = $1
		ORDER BY table_schema, table_name, privilege_type`, user)
	if err == nil {
		defer tableRows.Close()
		type key struct{ schema, table string }
		byTable := map[key][]string{}
		var order []key
		for tableRows.Next() {
			var schema, table, priv string
			if err := tableRows.Scan(&schema, &table, &priv); err != nil {
				return nil, err
			}
			k := key{schema, table}
			if _, ok := byTable[k]; !ok {
				order = append(order, k)
			}
			byTable[k] = append(byTable[k], priv)
		}
		if err := tableRows.Err(); err != nil {
			return nil, err
		}
		for _, k := range order {
			out = append(out, fmt.Sprintf("GRANT %s ON TABLE %s TO %s",
				strings.Join(byTable[k], ", "),
				qualifiedTable(k.schema, k.table),
				quotePGIdent(user)))
		}
	}
	return out, nil
}

// SchemaGrants returns per-role privileges on the connected database and public schema.
func (pg *Postgres) SchemaGrants(ctx context.Context, p ConnParams, database string) ([]SchemaGrant, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT role_name, privilege_type FROM (
			SELECT r.rolname AS role_name, p.priv AS privilege_type
			FROM pg_roles r
			CROSS JOIN (VALUES ('CONNECT'), ('CREATE'), ('TEMPORARY')) AS p(priv)
			WHERE r.rolcanlogin AND r.rolname NOT LIKE 'pg\_%'
			  AND has_database_privilege(r.oid, current_database(), p.priv)
			UNION ALL
			SELECT grantee, privilege_type
			FROM information_schema.schema_privileges
			WHERE schema_name = 'public'
			UNION ALL
			SELECT grantee, privilege_type
			FROM information_schema.table_privileges
			WHERE table_schema = 'public'
		) g
		ORDER BY role_name, privilege_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byRole := map[string]*SchemaGrant{}
	var order []string
	for rows.Next() {
		var role, priv string
		if err := rows.Scan(&role, &priv); err != nil {
			return nil, err
		}
		g, ok := byRole[role]
		if !ok {
			g = &SchemaGrant{User: role, Host: ""}
			byRole[role] = g
			order = append(order, role)
		}
		g.Privileges = append(g.Privileges, priv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]SchemaGrant, 0, len(order))
	for _, role := range order {
		sort.Strings(byRole[role].Privileges)
		seen := map[string]struct{}{}
		uniq := make([]string, 0, len(byRole[role].Privileges))
		for _, priv := range byRole[role].Privileges {
			if _, dup := seen[priv]; dup {
				continue
			}
			seen[priv] = struct{}{}
			uniq = append(uniq, priv)
		}
		byRole[role].Privileges = uniq
		out = append(out, *byRole[role])
	}
	return out, nil
}

// Grant maps MariaDB-style privilege names to PostgreSQL GRANT statements.
func (pg *Postgres) Grant(ctx context.Context, p ConnParams, user, host, database string, privileges []string) error {
	if !validPGRole(user) {
		return fmt.Errorf("invalid role name")
	}
	if !identRe.MatchString(database) {
		return fmt.Errorf("invalid database name")
	}
	privs, err := validatePrivileges(privileges)
	if err != nil {
		return err
	}
	stmts := postgresGrantStmts(database, user, privs)
	if len(stmts) == 0 {
		return fmt.Errorf("no applicable PostgreSQL privileges in request")
	}

	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	for _, stmt := range stmts {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// Revoke removes database, schema and table privileges from a role.
func (pg *Postgres) Revoke(ctx context.Context, p ConnParams, user, host, database string) error {
	if !validPGRole(user) {
		return fmt.Errorf("invalid role name")
	}
	if !identRe.MatchString(database) {
		return fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	role := quotePGIdent(user)
	stmts := []string{
		"REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM " + role,
		"REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM " + role,
		"REVOKE ALL PRIVILEGES ON SCHEMA public FROM " + role,
		fmt.Sprintf("REVOKE ALL PRIVILEGES ON DATABASE %s FROM %s", quoteIdent(database), role),
	}
	for _, stmt := range stmts {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// postgresGrantStmts translates the shared privilege allowlist into PostgreSQL GRANTs.
func postgresGrantStmts(database, role string, privs []string) []string {
	qRole := quotePGIdent(role)
	qDB := quoteIdent(database)

	dbPrivs := map[string]struct{}{}
	schemaPrivs := map[string]struct{}{}
	tablePrivs := map[string]struct{}{}

	for _, p := range privs {
		switch p {
		case "ALL PRIVILEGES":
			dbPrivs["CONNECT"] = struct{}{}
			dbPrivs["CREATE"] = struct{}{}
			schemaPrivs["USAGE"] = struct{}{}
			schemaPrivs["CREATE"] = struct{}{}
			for _, tp := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"} {
				tablePrivs[tp] = struct{}{}
			}
		case "SELECT", "INSERT", "UPDATE", "DELETE", "REFERENCES", "TRIGGER":
			tablePrivs[p] = struct{}{}
		case "CREATE", "CREATE TEMPORARY TABLES":
			schemaPrivs["CREATE"] = struct{}{}
			dbPrivs["CREATE"] = struct{}{}
		case "DROP", "ALTER", "INDEX":
			tablePrivs["REFERENCES"] = struct{}{}
		case "LOCK TABLES", "EXECUTE", "CREATE ROUTINE", "ALTER ROUTINE", "EVENT",
			"CREATE VIEW", "SHOW VIEW":
			// No direct PostgreSQL equivalent at schema scope; USAGE covers schema access.
			schemaPrivs["USAGE"] = struct{}{}
		}
	}

	var stmts []string
	if len(dbPrivs) > 0 {
		stmts = append(stmts, fmt.Sprintf("GRANT %s ON DATABASE %s TO %s",
			joinSet(dbPrivs), qDB, qRole))
	}
	if len(tablePrivs) > 0 {
		schemaPrivs["USAGE"] = struct{}{}
	}
	if len(schemaPrivs) > 0 {
		stmts = append(stmts, fmt.Sprintf("GRANT %s ON SCHEMA public TO %s",
			joinSet(schemaPrivs), qRole))
	}
	if len(tablePrivs) > 0 {
		stmts = append(stmts, fmt.Sprintf("GRANT %s ON ALL TABLES IN SCHEMA public TO %s",
			joinSet(tablePrivs), qRole))
	}
	return stmts
}

func joinSet(m map[string]struct{}) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// ListTables returns user table metadata for a database.
func (pg *Postgres) ListTables(ctx context.Context, p ConnParams, database string) ([]TableInfo, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT c.relname,
		       COALESCE(s.n_live_tup, 0),
		       COALESCE(pg_relation_size(c.oid), 0),
		       COALESCE(pg_indexes_size(c.oid), 0),
		       COALESCE(obj_description(c.oid, 'pg_class'), '')
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND c.relkind = 'r'
		ORDER BY c.relname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.RowCount, &t.DataBytes, &t.IndexBytes, &t.Comment); err != nil {
			return nil, err
		}
		t.Engine = "postgres"
		out = append(out, t)
	}
	return out, rows.Err()
}

// TableRows returns one page of a table's data with stringified values.
func (pg *Postgres) TableRows(ctx context.Context, p ConnParams, database, table string, limit, offset int) (*RowsPage, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	if !validTableName(table) {
		return nil, fmt.Errorf("invalid table name")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	schema, err := pg.tableSchema(ctx, conn, table)
	if err != nil {
		return nil, fmt.Errorf("table %q not found", table)
	}

	var total int64
	_ = conn.QueryRow(ctx, `
		SELECT COALESCE(c.reltuples::bigint, 0)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind = 'r'`,
		schema, table).Scan(&total)

	sqlText := fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d",
		qualifiedTable(schema, table), limit, offset)
	rows, err := conn.Query(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fds := rows.FieldDescriptions()
	cols := make([]string, len(fds))
	for i, fd := range fds {
		cols[i] = string(fd.Name)
	}
	page := &RowsPage{Columns: cols, Rows: [][]*string{}, Total: total}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make([]*string, len(vals))
		for i, v := range vals {
			row[i] = stringifyCell(v, 1024)
		}
		page.Rows = append(page.Rows, row)
	}
	return page, rows.Err()
}

// TableSchema returns a table's columns, indexes and reconstructed DDL.
func (pg *Postgres) TableSchema(ctx context.Context, p ConnParams, database, table string) (*TableSchema, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	if !validTableName(table) {
		return nil, fmt.Errorf("invalid table name")
	}
	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	schema, err := pg.tableSchema(ctx, conn, table)
	if err != nil {
		return nil, fmt.Errorf("table %q not found", table)
	}

	out := &TableSchema{Table: table, Columns: []ColumnInfo{}, Indexes: []IndexInfo{}}

	colRows, err := conn.Query(ctx, `
		SELECT column_name, data_type, is_nullable, column_default,
		       CASE WHEN EXISTS (
		         SELECT 1 FROM information_schema.table_constraints tc
		         JOIN information_schema.key_column_usage kcu
		           ON tc.constraint_name = kcu.constraint_name
		          AND tc.table_schema = kcu.table_schema
		         WHERE tc.constraint_type = 'PRIMARY KEY'
		           AND tc.table_schema = $1 AND tc.table_name = $2
		           AND kcu.column_name = c.column_name
		       ) THEN 'PRI' ELSE '' END AS col_key
		FROM information_schema.columns c
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`, schema, table)
	if err != nil {
		return nil, err
	}
	defer colRows.Close()
	for colRows.Next() {
		var c ColumnInfo
		var nullable string
		var def *string
		if err := colRows.Scan(&c.Name, &c.Type, &nullable, &def, &c.Key); err != nil {
			return nil, err
		}
		c.Nullable = nullable == "YES"
		c.Default = def
		out.Columns = append(out.Columns, c)
	}
	if err := colRows.Err(); err != nil {
		return nil, err
	}
	if len(out.Columns) == 0 {
		return nil, fmt.Errorf("table %q not found", table)
	}

	idxRows, err := conn.Query(ctx, `
		SELECT indexname, indexdef FROM pg_indexes
		WHERE schemaname = $1 AND tablename = $2
		ORDER BY indexname`, schema, table)
	if err != nil {
		return nil, err
	}
	defer idxRows.Close()
	for idxRows.Next() {
		var name, def string
		if err := idxRows.Scan(&name, &def); err != nil {
			return nil, err
		}
		unique := strings.Contains(strings.ToUpper(def), "UNIQUE")
		out.Indexes = append(out.Indexes, IndexInfo{Name: name, Unique: unique, Type: "btree", Columns: []string{}})
	}
	if err := idxRows.Err(); err != nil {
		return nil, err
	}

	out.DDL = buildPostgresDDL(schema, table, out.Columns)
	return out, nil
}

func buildPostgresDDL(schema, table string, cols []ColumnInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "CREATE TABLE %s (\n", qualifiedTable(schema, table))
	for i, c := range cols {
		if i > 0 {
			b.WriteString(",\n")
		}
		fmt.Fprintf(&b, "  %s %s", quotePGIdent(c.Name), c.Type)
		if c.Key == "PRI" {
			b.WriteString(" PRIMARY KEY")
		} else if !c.Nullable {
			b.WriteString(" NOT NULL")
		}
		if c.Default != nil && *c.Default != "" {
			fmt.Fprintf(&b, " DEFAULT %s", *c.Default)
		}
	}
	b.WriteString("\n);")
	return b.String()
}

// Query runs a single ad-hoc statement.
func (pg *Postgres) Query(ctx context.Context, p ConnParams, database, sqlText string, limit int, allowWrite bool) (*QueryResult, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return nil, fmt.Errorf("query is empty")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	readOnly := isReadOnlyStmt(sqlText)
	if !readOnly && !allowWrite {
		return nil, fmt.Errorf("this looks like a write statement; enable writes (requires database:write) to run it")
	}

	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	start := time.Now()
	res := &QueryResult{Columns: []string{}, Rows: [][]*string{}, ReadOnly: readOnly}

	if !readOnly {
		tag, err := conn.Exec(ctx, sqlText)
		if err != nil {
			return nil, err
		}
		res.RowsAffected = tag.RowsAffected()
		res.DurationMS = time.Since(start).Milliseconds()
		return res, nil
	}

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fds := rows.FieldDescriptions()
	for _, fd := range fds {
		res.Columns = append(res.Columns, string(fd.Name))
	}
	for rows.Next() {
		if len(res.Rows) == limit {
			res.Truncated = true
			break
		}
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make([]*string, len(vals))
		for i, v := range vals {
			row[i] = stringifyCell(v, 1024)
		}
		res.Rows = append(res.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	res.RowCount = len(res.Rows)
	res.DurationMS = time.Since(start).Milliseconds()
	return res, nil
}

// ExportCSV streams a whole table or a read-only query to w as CSV.
func (pg *Postgres) ExportCSV(ctx context.Context, p ConnParams, database, table, query string, w io.Writer, onStart func()) (int64, error) {
	if !identRe.MatchString(database) {
		return 0, fmt.Errorf("invalid database name")
	}
	var sqlText string
	switch {
	case table != "":
		if !validTableName(table) {
			return 0, fmt.Errorf("invalid table name")
		}
		conn, err := pg.connect(ctx, p, database)
		if err != nil {
			return 0, err
		}
		schema, err := pg.tableSchema(ctx, conn, table)
		conn.Close(ctx)
		if err != nil {
			return 0, fmt.Errorf("table %q not found", table)
		}
		sqlText = "SELECT * FROM " + qualifiedTable(schema, table)
	case strings.TrimSpace(query) != "":
		sqlText = strings.TrimSpace(query)
		if !isReadOnlyStmt(sqlText) {
			return 0, fmt.Errorf("only read-only statements can be exported")
		}
	default:
		return 0, fmt.Errorf("either a table or a query is required")
	}

	conn, err := pg.connect(ctx, p, database)
	if err != nil {
		return 0, err
	}
	defer conn.Close(ctx)

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, sqlText)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	fds := rows.FieldDescriptions()
	cols := make([]string, len(fds))
	for i, fd := range fds {
		cols[i] = string(fd.Name)
	}

	if onStart != nil {
		onStart()
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return 0, err
	}

	var count int64
	for rows.Next() {
		if count >= maxExportRows {
			break
		}
		vals, err := rows.Values()
		if err != nil {
			cw.Flush()
			return count, err
		}
		record := make([]string, len(vals))
		for i, v := range vals {
			if s := stringifyCell(v, 0); s != nil {
				record[i] = *s
			}
		}
		if err := cw.Write(record); err != nil {
			return count, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		cw.Flush()
		return count, err
	}
	cw.Flush()
	return count, cw.Error()
}
