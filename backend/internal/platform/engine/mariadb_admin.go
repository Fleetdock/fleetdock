package engine

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

var _ Admin = (*MariaDB)(nil) // serves both mariadb and mysql engines

// escapeMySQLString escapes a value for a single-quoted MySQL/MariaDB string
// literal. Because these engines treat backslash as an escape character inside
// string literals by default, escaping only the quote is insufficient: a
// trailing or embedded backslash could consume the closing quote and let the
// value break out of the literal. Backslashes must be doubled first, then
// single quotes.
func escapeMySQLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "'", "''")
	return s
}

// quoteAccount renders 'user'@'host' with the parts escaped. user/host are also
// validated by validAccountPart (which rejects backslashes and quotes) before
// reaching here; the escaping is defense in depth.
func quoteAccount(user, host string) string {
	if host == "" {
		host = "%"
	}
	return fmt.Sprintf("'%s'@'%s'", escapeMySQLString(user), escapeMySQLString(host))
}

func validAccountPart(s string, max int) bool {
	if s == "" || len(s) > max {
		return false
	}
	return !strings.ContainsAny(s, "'\"`\\\n\r\x00")
}

func (m *MariaDB) validateAccount(user, host string) error {
	if !validAccountPart(user, 32) {
		return fmt.Errorf("invalid username")
	}
	if host != "" && !validAccountPart(host, 255) {
		return fmt.Errorf("invalid host")
	}
	return nil
}

// ListDBUsers returns non-system accounts.
func (m *MariaDB) ListDBUsers(ctx context.Context, p ConnParams) ([]DBUser, error) {
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT User, Host FROM mysql.user
		WHERE User NOT IN ('mariadb.sys','mysql','root@localhost') AND User <> ''
		ORDER BY User, Host`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DBUser
	for rows.Next() {
		var u DBUser
		if err := rows.Scan(&u.User, &u.Host); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CreateDBUser creates an account.
func (m *MariaDB) CreateDBUser(ctx context.Context, p ConnParams, user, host, password string) error {
	if err := m.validateAccount(user, host); err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}
	db, err := m.open(p)
	if err != nil {
		return err
	}
	defer db.Close()
	pw := escapeMySQLString(password)
	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"CREATE USER %s IDENTIFIED BY '%s'", quoteAccount(user, host), pw))
	return err
}

// DropDBUser removes an account.
func (m *MariaDB) DropDBUser(ctx context.Context, p ConnParams, user, host string) error {
	if err := m.validateAccount(user, host); err != nil {
		return err
	}
	db, err := m.open(p)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, "DROP USER IF EXISTS "+quoteAccount(user, host))
	return err
}

// UserGrants returns the raw SHOW GRANTS statements for an account.
func (m *MariaDB) UserGrants(ctx context.Context, p ConnParams, user, host string) ([]string, error) {
	if err := m.validateAccount(user, host); err != nil {
		return nil, err
	}
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SHOW GRANTS FOR "+quoteAccount(user, host))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SchemaGrants returns per-account privileges on one schema.
func (m *MariaDB) SchemaGrants(ctx context.Context, p ConnParams, database string) ([]SchemaGrant, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT GRANTEE, PRIVILEGE_TYPE
		FROM information_schema.SCHEMA_PRIVILEGES
		WHERE TABLE_SCHEMA = ?
		ORDER BY GRANTEE, PRIVILEGE_TYPE`, database)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byAccount := map[string]*SchemaGrant{}
	var order []string
	for rows.Next() {
		var grantee, priv string
		if err := rows.Scan(&grantee, &priv); err != nil {
			return nil, err
		}
		user, host := parseGrantee(grantee)
		key := user + "@" + host
		g, ok := byAccount[key]
		if !ok {
			g = &SchemaGrant{User: user, Host: host}
			byAccount[key] = g
			order = append(order, key)
		}
		g.Privileges = append(g.Privileges, priv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]SchemaGrant, 0, len(order))
	for _, k := range order {
		sort.Strings(byAccount[k].Privileges)
		out = append(out, *byAccount[k])
	}
	return out, nil
}

// parseGrantee splits `'user'@'host'` into its parts.
func parseGrantee(g string) (user, host string) {
	parts := strings.SplitN(g, "@", 2)
	user = strings.Trim(parts[0], "'`\"")
	if len(parts) == 2 {
		host = strings.Trim(parts[1], "'`\"")
	}
	return user, host
}

// Grant grants schema-level privileges to an account.
func (m *MariaDB) Grant(ctx context.Context, p ConnParams, user, host, database string, privileges []string) error {
	if err := m.validateAccount(user, host); err != nil {
		return err
	}
	if !identRe.MatchString(database) {
		return fmt.Errorf("invalid database name")
	}
	privs, err := validatePrivileges(privileges)
	if err != nil {
		return err
	}
	db, err := m.open(p)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"GRANT %s ON `%s`.* TO %s", strings.Join(privs, ", "), database, quoteAccount(user, host)))
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return err
}

// Revoke removes all schema-level privileges from an account.
func (m *MariaDB) Revoke(ctx context.Context, p ConnParams, user, host, database string) error {
	if err := m.validateAccount(user, host); err != nil {
		return err
	}
	if !identRe.MatchString(database) {
		return fmt.Errorf("invalid database name")
	}
	db, err := m.open(p)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"REVOKE ALL PRIVILEGES ON `%s`.* FROM %s", database, quoteAccount(user, host)))
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return err
}

func validatePrivileges(privs []string) ([]string, error) {
	if len(privs) == 0 {
		return nil, fmt.Errorf("at least one privilege is required")
	}
	valid := make(map[string]struct{}, len(GrantablePrivileges))
	for _, p := range GrantablePrivileges {
		valid[p] = struct{}{}
	}
	out := make([]string, 0, len(privs))
	seen := map[string]struct{}{}
	for _, p := range privs {
		p = strings.ToUpper(strings.TrimSpace(p))
		if _, ok := valid[p]; !ok {
			return nil, fmt.Errorf("unknown privilege %q", p)
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out, nil
}

// ListTables returns table metadata for a database.
func (m *MariaDB) ListTables(ctx context.Context, p ConnParams, database string) ([]TableInfo, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT TABLE_NAME, COALESCE(ENGINE,''), COALESCE(TABLE_ROWS,0),
		       COALESCE(DATA_LENGTH,0), COALESCE(INDEX_LENGTH,0), COALESCE(TABLE_COMMENT,'')
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'
		ORDER BY TABLE_NAME`, database)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.Engine, &t.RowCount, &t.DataBytes, &t.IndexBytes, &t.Comment); err != nil {
			return nil, err
		}
		// In MySQL/MariaDB a schema is a database, so the two always match.
		t.Schema = database
		out = append(out, t)
	}
	return out, rows.Err()
}

// TableRows returns one page of a table's data with stringified values.
func (m *MariaDB) TableRows(ctx context.Context, p ConnParams, database, table string, limit, offset int) (*RowsPage, error) {
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

	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var total int64
	_ = db.QueryRowContext(ctx, `
		SELECT COALESCE(TABLE_ROWS,0) FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`, database, table).Scan(&total)

	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		"SELECT * FROM `%s`.`%s` LIMIT %d OFFSET %d", database, table, limit, offset))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	page := &RowsPage{Columns: cols, Rows: [][]*string{}, Total: total}
	for rows.Next() {
		raw := make([]sql.RawBytes, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make([]*string, len(cols))
		for i, b := range raw {
			if b == nil {
				continue
			}
			s := string(b)
			if len(s) > 1024 {
				s = s[:1024] + "…"
			}
			row[i] = &s
		}
		page.Rows = append(page.Rows, row)
	}
	return page, rows.Err()
}

// TableSchema returns a table's columns, indexes and CREATE DDL.
func (m *MariaDB) TableSchema(ctx context.Context, p ConnParams, database, table string) (*TableSchema, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	if !validTableName(table) {
		return nil, fmt.Errorf("invalid table name")
	}
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	out := &TableSchema{Table: table, Columns: []ColumnInfo{}, Indexes: []IndexInfo{}}

	colRows, err := db.QueryContext(ctx, `
		SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY,
		       COLUMN_DEFAULT, EXTRA, COLUMN_COMMENT
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION`, database, table)
	if err != nil {
		return nil, err
	}
	defer colRows.Close()
	for colRows.Next() {
		var (
			c        ColumnInfo
			nullable string
			def      sql.NullString
		)
		if err := colRows.Scan(&c.Name, &c.Type, &nullable, &c.Key, &def, &c.Extra, &c.Comment); err != nil {
			return nil, err
		}
		c.Nullable = nullable == "YES"
		if def.Valid {
			d := def.String
			c.Default = &d
		}
		out.Columns = append(out.Columns, c)
	}
	if err := colRows.Err(); err != nil {
		return nil, err
	}
	if len(out.Columns) == 0 {
		return nil, fmt.Errorf("table %q not found", table)
	}

	idxRows, err := db.QueryContext(ctx, `
		SELECT INDEX_NAME, NON_UNIQUE, INDEX_TYPE, COLUMN_NAME
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY INDEX_NAME, SEQ_IN_INDEX`, database, table)
	if err != nil {
		return nil, err
	}
	defer idxRows.Close()
	byIndex := map[string]*IndexInfo{}
	var idxOrder []string
	for idxRows.Next() {
		var (
			name, idxType, col string
			nonUnique          int
		)
		if err := idxRows.Scan(&name, &nonUnique, &idxType, &col); err != nil {
			return nil, err
		}
		idx, ok := byIndex[name]
		if !ok {
			idx = &IndexInfo{Name: name, Unique: nonUnique == 0, Type: idxType}
			byIndex[name] = idx
			idxOrder = append(idxOrder, name)
		}
		idx.Columns = append(idx.Columns, col)
	}
	if err := idxRows.Err(); err != nil {
		return nil, err
	}
	for _, name := range idxOrder {
		out.Indexes = append(out.Indexes, *byIndex[name])
	}

	// SHOW CREATE TABLE returns (Table, Create Table).
	var name, ddl string
	if err := db.QueryRowContext(ctx, fmt.Sprintf(
		"SHOW CREATE TABLE `%s`.`%s`", database, table)).Scan(&name, &ddl); err == nil {
		out.DDL = ddl
	}
	return out, nil
}

// Query runs a single ad-hoc statement against the connection's default schema.
func (m *MariaDB) Query(ctx context.Context, p ConnParams, database, sqlText string, limit int, allowWrite bool) (*QueryResult, error) {
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

	p.Database = database
	db, err := m.open(p)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	start := time.Now()
	res := &QueryResult{Columns: []string{}, Rows: [][]*string{}, ReadOnly: readOnly}

	if !readOnly {
		out, err := db.ExecContext(ctx, sqlText)
		if err != nil {
			return nil, err
		}
		res.RowsAffected, _ = out.RowsAffected()
		res.DurationMS = time.Since(start).Milliseconds()
		return res, nil
	}

	// Read-only path: run inside a READ ONLY transaction so a statement that
	// slips past classification but attempts a write is refused by the engine.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	res.Columns = cols
	for rows.Next() {
		if len(res.Rows) == limit {
			res.Truncated = true
			break
		}
		row, err := scanStringRow(rows, len(cols), 1024)
		if err != nil {
			return nil, err
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
func (m *MariaDB) ExportCSV(ctx context.Context, p ConnParams, database, table, query string, w io.Writer, onStart func()) (int64, error) {
	if !identRe.MatchString(database) {
		return 0, fmt.Errorf("invalid database name")
	}
	var sqlText string
	switch {
	case table != "":
		if !validTableName(table) {
			return 0, fmt.Errorf("invalid table name")
		}
		sqlText = fmt.Sprintf("SELECT * FROM `%s`.`%s`", database, table)
	case strings.TrimSpace(query) != "":
		sqlText = strings.TrimSpace(query)
		if !isReadOnlyStmt(sqlText) {
			return 0, fmt.Errorf("only read-only statements can be exported")
		}
	default:
		return 0, fmt.Errorf("either a table or a query is required")
	}

	p.Database = database
	db, err := m.open(p)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, sqlText)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}

	// The result set opened cleanly; from here bytes may be written, so let the
	// caller commit to the success response before we emit the header row.
	if onStart != nil {
		onStart()
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return 0, err
	}

	record := make([]string, len(cols))
	var count int64
	for rows.Next() {
		if count >= maxExportRows {
			break
		}
		row, err := scanStringRow(rows, len(cols), 0)
		if err != nil {
			cw.Flush()
			return count, err
		}
		for i, v := range row {
			if v == nil {
				record[i] = ""
			} else {
				record[i] = *v
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
