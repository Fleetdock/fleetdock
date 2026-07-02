package engine

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

var _ Admin = (*MariaDB)(nil)

// quoteAccount renders 'user'@'host' with single quotes escaped.
func quoteAccount(user, host string) string {
	esc := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	if host == "" {
		host = "%"
	}
	return fmt.Sprintf("'%s'@'%s'", esc(user), esc(host))
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
	pw := strings.ReplaceAll(password, "'", "''")
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
		out = append(out, t)
	}
	return out, rows.Err()
}

// TableRows returns one page of a table's data with stringified values.
func (m *MariaDB) TableRows(ctx context.Context, p ConnParams, database, table string, limit, offset int) (*RowsPage, error) {
	if !identRe.MatchString(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	// Table names allow more characters than database names; still reject quoting hazards.
	if table == "" || len(table) > 64 || strings.ContainsAny(table, "`'\"\\\n\r\x00") {
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
