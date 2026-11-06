package engine

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// chUserRe bounds ClickHouse account names to what Fleetdock manages: plain
// identifiers, which also need no quoting in SHOW GRANTS output.
var chUserRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,63}$`)

// chHostRe is a MySQL-style host pattern ('%', '10.0.%', 'app.example.com').
var chHostRe = regexp.MustCompile(`^[A-Za-z0-9_.:%-]{1,255}$`)

func validCHUser(user string) error {
	if !chUserRe.MatchString(user) {
		return fmt.Errorf("invalid user name %q", user)
	}
	return nil
}

// chHostClause maps a MySQL-style host onto ClickHouse's HOST clause.
// ClickHouse accounts have no host component; the pattern becomes a
// connection restriction instead.
func chHostClause(host string) (string, error) {
	switch host {
	case "", "%":
		return "HOST ANY", nil
	case "localhost":
		return "HOST LOCAL", nil
	}
	if !chHostRe.MatchString(host) {
		return "", fmt.Errorf("invalid host %q", host)
	}
	return "HOST LIKE " + quoteCHString(host), nil
}

// chPrivileges maps the grant names the dashboard offers onto ClickHouse
// privileges. Names ClickHouse has no equivalent for are refused rather than
// silently dropped.
var chPrivileges = map[string][]string{
	"ALL PRIVILEGES":    {"ALL"},
	"ALL":               {"ALL"},
	"SELECT":            {"SELECT"},
	"INSERT":            {"INSERT"},
	"UPDATE":            {"ALTER UPDATE"},
	"DELETE":            {"ALTER DELETE"},
	"CREATE":            {"CREATE TABLE", "CREATE VIEW", "CREATE DICTIONARY"},
	"DROP":              {"DROP TABLE", "DROP VIEW", "DROP DICTIONARY"},
	"ALTER":             {"ALTER"},
	"INDEX":             {"ALTER INDEX"},
	"CREATE VIEW":       {"CREATE VIEW"},
	"SHOW VIEW":         {"SHOW"},
	"SHOW":              {"SHOW"},
	"TRUNCATE":          {"TRUNCATE"},
	"OPTIMIZE":          {"OPTIMIZE"},
	"CREATE TABLE":      {"CREATE TABLE"},
	"DROP TABLE":        {"DROP TABLE"},
	"ALTER UPDATE":      {"ALTER UPDATE"},
	"ALTER DELETE":      {"ALTER DELETE"},
	"CREATE DICTIONARY": {"CREATE DICTIONARY"},
}

func chGrantList(privileges []string) (string, error) {
	if len(privileges) == 0 {
		return "", fmt.Errorf("at least one privilege is required")
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range privileges {
		mapped, ok := chPrivileges[strings.ToUpper(strings.TrimSpace(p))]
		if !ok {
			return "", fmt.Errorf("privilege %q is not supported on ClickHouse", p)
		}
		for _, m := range mapped {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return strings.Join(out, ", "), nil
}

// ListDBUsers lists the server's accounts. Host is "%" for accounts that may
// connect from anywhere, else the first host restriction.
func (c *ClickHouse) ListDBUsers(ctx context.Context, p ConnParams) ([]DBUser, error) {
	rows, err := c.rows(ctx, p, chRequest{query: `
		SELECT name,
		       multiIf(has(host_ip, '::/0'), '%',
		               notEmpty(host_names_like), host_names_like[1],
		               notEmpty(host_names), host_names[1],
		               notEmpty(host_ip), host_ip[1],
		               notEmpty(host_names_regexp), host_names_regexp[1], '')
		FROM system.users ORDER BY name`})
	if err != nil {
		return nil, err
	}
	out := make([]DBUser, 0, len(rows))
	for _, r := range rows {
		out = append(out, DBUser{User: str(r[0]), Host: str(r[1])})
	}
	return out, nil
}

// CreateDBUser creates an account with a SHA-256 password.
func (c *ClickHouse) CreateDBUser(ctx context.Context, p ConnParams, user, host, password string) error {
	if err := validCHUser(user); err != nil {
		return err
	}
	hostClause, err := chHostClause(host)
	if err != nil {
		return err
	}
	return c.exec(ctx, p, chRequest{query: fmt.Sprintf("CREATE USER %s IDENTIFIED WITH sha256_password BY %s %s",
		quoteCHIdent(user), quoteCHString(password), hostClause)})
}

// DropDBUser drops an account; host does not apply on ClickHouse.
func (c *ClickHouse) DropDBUser(ctx context.Context, p ConnParams, user, _ string) error {
	if err := validCHUser(user); err != nil {
		return err
	}
	return c.exec(ctx, p, chRequest{query: "DROP USER IF EXISTS " + quoteCHIdent(user)})
}

// UserGrants returns the SHOW GRANTS statements for an account.
func (c *ClickHouse) UserGrants(ctx context.Context, p ConnParams, user, _ string) ([]string, error) {
	if err := validCHUser(user); err != nil {
		return nil, err
	}
	rows, err := c.rows(ctx, p, chRequest{query: "SHOW GRANTS FOR " + quoteCHIdent(user)})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, str(r[0]))
	}
	return out, nil
}

// SchemaGrants returns each account's privileges on one database (grants on
// the whole database, not on single tables).
func (c *ClickHouse) SchemaGrants(ctx context.Context, p ConnParams, database string) ([]SchemaGrant, error) {
	if !validDBName(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	rows, err := c.rows(ctx, p, chRequest{
		query: `
			SELECT user_name, access_type
			FROM system.grants
			WHERE database = {db:String} AND table IS NULL AND user_name IS NOT NULL AND NOT is_partial_revoke
			ORDER BY user_name, access_type`,
		params: map[string]string{"db": database},
	})
	if err != nil {
		return nil, err
	}
	byUser := map[string]*SchemaGrant{}
	var order []string
	for _, r := range rows {
		u := str(r[0])
		g, ok := byUser[u]
		if !ok {
			g = &SchemaGrant{User: u, Host: "%"}
			byUser[u] = g
			order = append(order, u)
		}
		g.Privileges = append(g.Privileges, str(r[1]))
	}
	out := make([]SchemaGrant, 0, len(order))
	for _, u := range order {
		sort.Strings(byUser[u].Privileges)
		out = append(out, *byUser[u])
	}
	return out, nil
}

// Grant grants database-level privileges to an account.
func (c *ClickHouse) Grant(ctx context.Context, p ConnParams, user, _, database string, privileges []string) error {
	if err := validCHUser(user); err != nil {
		return err
	}
	if !validDBName(database) {
		return fmt.Errorf("invalid database name")
	}
	privs, err := chGrantList(privileges)
	if err != nil {
		return err
	}
	return c.exec(ctx, p, chRequest{query: fmt.Sprintf("GRANT %s ON %s.* TO %s", privs, quoteCHIdent(database), quoteCHIdent(user))})
}

// Revoke removes all of an account's privileges on a database.
func (c *ClickHouse) Revoke(ctx context.Context, p ConnParams, user, _, database string) error {
	if err := validCHUser(user); err != nil {
		return err
	}
	if !validDBName(database) {
		return fmt.Errorf("invalid database name")
	}
	return c.exec(ctx, p, chRequest{query: fmt.Sprintf("REVOKE ALL ON %s.* FROM %s", quoteCHIdent(database), quoteCHIdent(user))})
}

// chProfileGrants are the access profiles in ClickHouse privileges.
var chProfileGrants = map[AccessProfile]string{
	ProfileReadonly: "SELECT, SHOW",
	ProfileReadWrite: "SELECT, INSERT, ALTER, CREATE TABLE, CREATE VIEW, CREATE DICTIONARY, " +
		"DROP TABLE, DROP VIEW, DROP DICTIONARY, TRUNCATE, OPTIMIZE, SHOW",
	ProfileAdmin: "ALL",
}

// ApplyAccessProfile grants a profile on one database. A readonly account is
// also pinned to readonly=2 with a constraint it cannot lift, so the server
// refuses every write it attempts, whatever its grants say.
func (c *ClickHouse) ApplyAccessProfile(ctx context.Context, p ConnParams, user, _, database string, profile AccessProfile) error {
	privs, ok := chProfileGrants[profile]
	if !ok {
		return fmt.Errorf("unsupported access profile %q", profile)
	}
	if err := validCHUser(user); err != nil {
		return err
	}
	if !validDBName(database) {
		return fmt.Errorf("invalid database name")
	}
	u := quoteCHIdent(user)
	if err := c.exec(ctx, p, chRequest{query: fmt.Sprintf("GRANT %s ON %s.* TO %s", privs, quoteCHIdent(database), u)}); err != nil {
		return err
	}
	if profile == ProfileReadonly {
		return c.exec(ctx, p, chRequest{query: "ALTER USER " + u + " SETTINGS readonly = 2 READONLY"})
	}
	return c.exec(ctx, p, chRequest{query: "ALTER USER " + u + " DROP ALL SETTINGS"})
}

// setPassword changes an account's password.
func (c *ClickHouse) setPassword(ctx context.Context, p ConnParams, user, password string) error {
	if err := validCHUser(user); err != nil {
		return err
	}
	return c.exec(ctx, p, chRequest{query: fmt.Sprintf("ALTER USER %s IDENTIFIED WITH sha256_password BY %s",
		quoteCHIdent(user), quoteCHString(password))})
}

// ListTables lists a database's tables and views with their size.
func (c *ClickHouse) ListTables(ctx context.Context, p ConnParams, database string) ([]TableInfo, error) {
	if !validDBName(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	rows, err := c.rows(ctx, p, chRequest{
		query: `
			SELECT t.name, t.engine, ifNull(t.total_rows, 0), ifNull(t.total_bytes, 0),
			       ifNull(k.bytes, 0), t.comment
			FROM system.tables t
			LEFT JOIN (
				SELECT table, sum(primary_key_bytes_in_memory) AS bytes
				FROM system.parts WHERE database = {db:String} AND active GROUP BY table
			) k ON k.table = t.name
			WHERE t.database = {db:String} AND NOT t.is_temporary AND t.engine NOT IN ({views:Array(String)})
			ORDER BY t.name
			SETTINGS join_use_nulls = 1`,
		params: map[string]string{"db": database, "views": chViewEnginesParam},
	})
	if err != nil {
		return nil, err
	}
	out := make([]TableInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, TableInfo{
			Name: str(r[0]), Schema: database, Engine: str(r[1]),
			RowCount: i64(r[2]), DataBytes: i64(r[3]), IndexBytes: i64(r[4]), Comment: str(r[5]),
		})
	}
	return out, nil
}

// qualified returns `db`.`table` after validating both names.
func chQualified(database, table string) (string, error) {
	if !validDBName(database) {
		return "", fmt.Errorf("invalid database name")
	}
	if !validTableName(table) {
		return "", fmt.Errorf("invalid table name")
	}
	return quoteCHIdent(database) + "." + quoteCHIdent(table), nil
}

// TableRows returns one page of a table in its natural order.
func (c *ClickHouse) TableRows(ctx context.Context, p ConnParams, database, table string, limit, offset int) (*RowsPage, error) {
	qt, err := chQualified(database, table)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	ctx, cancel := withDefaultTimeout(ctx, chTimeout)
	defer cancel()
	resp, err := c.open(ctx, p, chRequest{
		query:    fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", qt, limit, offset),
		database: database, readOnly: true,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Close()
	cols, _, err := resp.header()
	if err != nil {
		return nil, err
	}
	page := &RowsPage{Columns: cols, Rows: [][]*string{}}
	for {
		row, ok, err := resp.row(1024)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		page.Rows = append(page.Rows, row)
	}
	page.Total, _ = c.estimatedRows(ctx, p, database, table)
	return page, nil
}

// estimatedRows is system.tables' row count (exact for MergeTree tables,
// unknown — 0 — for views and engines that do not track it).
func (c *ClickHouse) estimatedRows(ctx context.Context, p ConnParams, database, table string) (int64, error) {
	rows, err := c.rows(ctx, p, chRequest{
		query:  "SELECT ifNull(total_rows, 0) FROM system.tables WHERE database = {db:String} AND name = {t:String}",
		params: map[string]string{"db": database, "t": table},
	})
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return i64(rows[0][0]), nil
}

// TableSchema returns a table's columns, its primary key and data-skipping
// indexes, and its CREATE statement.
func (c *ClickHouse) TableSchema(ctx context.Context, p ConnParams, database, table string) (*TableSchema, error) {
	if _, err := chQualified(database, table); err != nil {
		return nil, err
	}
	params := map[string]string{"db": database, "t": table}
	cols, err := c.rows(ctx, p, chRequest{
		query: `
			SELECT name, type, default_kind, default_expression, comment, is_in_primary_key, is_in_sorting_key
			FROM system.columns WHERE database = {db:String} AND table = {t:String}
			ORDER BY position`,
		params: params,
	})
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("table %q not found", table)
	}
	out := &TableSchema{Table: table, Columns: make([]ColumnInfo, 0, len(cols)), Indexes: []IndexInfo{}}
	var pk []string
	for _, r := range cols {
		ci := ColumnInfo{Name: str(r[0]), Type: str(r[1]), Comment: str(r[4])}
		ci.Nullable = strings.HasPrefix(ci.Type, "Nullable(") || strings.HasPrefix(ci.Type, "LowCardinality(Nullable(")
		switch kind := str(r[2]); kind {
		case "DEFAULT":
			if v := r[3]; v != nil {
				ci.Default = v
			}
		case "MATERIALIZED", "ALIAS", "EPHEMERAL":
			ci.Extra = kind + " " + str(r[3])
		}
		switch {
		case str(r[5]) == "1":
			ci.Key = "PRI"
			pk = append(pk, ci.Name)
		case str(r[6]) == "1":
			ci.Key = "MUL"
		}
		out.Columns = append(out.Columns, ci)
	}
	if len(pk) > 0 {
		out.Indexes = append(out.Indexes, IndexInfo{Name: "PRIMARY", Columns: pk, Type: "PRIMARY KEY"})
	}
	idx, err := c.rows(ctx, p, chRequest{
		query:  "SELECT name, expr, type FROM system.data_skipping_indices WHERE database = {db:String} AND table = {t:String} ORDER BY name",
		params: params,
	})
	if err == nil {
		for _, r := range idx {
			out.Indexes = append(out.Indexes, IndexInfo{Name: str(r[0]), Columns: []string{str(r[1])}, Type: strings.ToUpper(str(r[2]))})
		}
	}
	ddl, err := c.rows(ctx, p, chRequest{
		query:  "SELECT create_table_query FROM system.tables WHERE database = {db:String} AND name = {t:String}",
		params: params,
	})
	if err == nil && len(ddl) > 0 {
		out.DDL = str(ddl[0][0])
	}
	return out, nil
}

// Query runs one console statement.
func (c *ClickHouse) Query(ctx context.Context, p ConnParams, database, sqlText string, limit int, allowWrite bool) (*QueryResult, error) {
	return singleResult(c.QueryBatch(ctx, p, database, []string{sqlText}, limit, allowWrite, nil))
}

// QueryBatch runs statements in order on one server session, so SET and
// temporary tables carry over between them. ClickHouse has no transactions:
// statements before a failing one stay applied.
//
// Each statement runs under the same numeric query id, reported through
// onSession, so cancelling the batch kills whichever statement is running.
func (c *ClickHouse) QueryBatch(ctx context.Context, p ConnParams, database string, stmts []string, limit int, allowWrite bool, onSession func(id int64)) ([]QueryResult, error) {
	if !validDBName(database) {
		return nil, fmt.Errorf("invalid database name")
	}
	if err := checkBatch(stmts, allowWrite); err != nil {
		return nil, err
	}
	limit = clampQueryLimit(limit)
	qid := newQueryID()
	if onSession != nil {
		onSession(qid)
	}
	session := fmt.Sprintf("fleetdock-%d", qid)
	results := make([]QueryResult, 0, len(stmts))
	for i, stmt := range stmts {
		res, err := c.runStatement(ctx, p, chRequest{
			query: strings.TrimSpace(stmt), database: database,
			sessionID: session, queryID: strconv.FormatInt(qid, 10),
		}, limit, allowWrite)
		if err != nil {
			return results, &BatchError{Index: i, Err: err}
		}
		results = append(results, *res)
	}
	return results, nil
}

// runStatement runs one statement and reads at most limit rows (and
// maxResultBytes) of its result. Reads in a read-only batch run with
// readonly=2, so the server refuses one that was misclassified and writes.
func (c *ClickHouse) runStatement(ctx context.Context, p ConnParams, req chRequest, limit int, allowWrite bool) (*QueryResult, error) {
	start := time.Now()
	readOnly := isReadOnlyStmt(req.query)
	res := &QueryResult{Columns: []string{}, Rows: [][]*string{}, ReadOnly: readOnly}
	if readOnly {
		req.readOnly = !allowWrite
		// Let the server stop producing rows soon after the limit.
		req.settings = map[string]string{
			"max_result_rows":      strconv.Itoa(limit + 1),
			"result_overflow_mode": "break",
		}
	}
	resp, err := c.open(ctx, p, req)
	if err != nil {
		return nil, err
	}
	defer resp.Close()
	if !readOnly {
		res.RowsAffected = resp.writtenRows()
	}

	if resp.format != "" && resp.format != chResultFormat {
		// The statement chose its own output format (FORMAT CSV, ...): show
		// its text one line per row.
		res.Columns = []string{"result"}
		size := 0
		for {
			line, err := resp.line()
			if err != nil {
				return nil, err
			}
			if line == nil {
				break
			}
			if len(res.Rows) == limit || size > maxResultBytes {
				res.Truncated = true
				break
			}
			s := string(line)
			if len(s) > 1024 {
				s = s[:1024] + "…"
			}
			size += len(s)
			res.Rows = append(res.Rows, []*string{&s})
		}
	} else {
		cols, _, err := resp.header()
		if err != nil {
			return nil, err
		}
		if cols != nil {
			res.Columns = cols
		}
		size := 0
		for {
			row, ok, err := resp.row(1024)
			if err != nil {
				return nil, err
			}
			if !ok {
				break
			}
			if len(res.Rows) == limit || size > maxResultBytes {
				res.Truncated = true
				break
			}
			size += rowBytes(row)
			res.Rows = append(res.Rows, row)
		}
	}
	res.RowCount = len(res.Rows)
	res.DurationMS = time.Since(start).Milliseconds()
	return res, nil
}

// ExportCSV streams a whole table or a read-only query to w as CSV.
func (c *ClickHouse) ExportCSV(ctx context.Context, p ConnParams, database, table, query string, w io.Writer, onStart func()) (int64, error) {
	if !validDBName(database) {
		return 0, fmt.Errorf("invalid database name")
	}
	var sqlText string
	switch {
	case table != "":
		qt, err := chQualified(database, table)
		if err != nil {
			return 0, err
		}
		sqlText = "SELECT * FROM " + qt
	case strings.TrimSpace(query) != "":
		sqlText = strings.TrimSpace(query)
		if !isReadOnlyStmt(sqlText) {
			return 0, fmt.Errorf("only read-only statements can be exported")
		}
	default:
		return 0, fmt.Errorf("either a table or a query is required")
	}
	return c.streamCSV(ctx, p, chRequest{query: sqlText, database: database}, w, onStart)
}

// streamCSV runs a read with readonly=2 and writes its rows to w as CSV, up
// to maxExportRows. NULL is written as an empty field, as on the other
// engines.
func (c *ClickHouse) streamCSV(ctx context.Context, p ConnParams, req chRequest, w io.Writer, onStart func()) (int64, error) {
	req.readOnly = true
	req.settings = map[string]string{
		"max_result_rows":      strconv.Itoa(maxExportRows + 1),
		"result_overflow_mode": "break",
	}
	resp, err := c.open(ctx, p, req)
	if err != nil {
		return 0, err
	}
	defer resp.Close()
	if resp.format != chResultFormat {
		return 0, fmt.Errorf("remove the FORMAT clause to export a query as CSV")
	}
	cols, _, err := resp.header()
	if err != nil {
		return 0, err
	}
	if onStart != nil {
		onStart()
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return 0, err
	}
	record := make([]string, len(cols))
	var count int64
	for count < maxExportRows {
		row, ok, err := resp.row(0)
		if err != nil {
			cw.Flush()
			return count, err
		}
		if !ok {
			break
		}
		for i, v := range row {
			record[i] = str(v)
		}
		if err := cw.Write(record); err != nil {
			return count, err
		}
		count++
	}
	cw.Flush()
	return count, cw.Error()
}
