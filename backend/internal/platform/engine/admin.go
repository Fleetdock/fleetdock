package engine

import (
	"context"
	"io"
)

// DBUser is a database-level account (not a control-plane user).
type DBUser struct {
	User string `json:"user"`
	Host string `json:"host"`
}

// SchemaGrant is one account's privileges on one schema.
type SchemaGrant struct {
	User       string   `json:"user"`
	Host       string   `json:"host"`
	Privileges []string `json:"privileges"`
}

// TableInfo describes a table within a database.
type TableInfo struct {
	Name string `json:"name"`
	// Schema is the namespace the table lives in. PostgreSQL databases hold
	// many schemas, so Name alone is ambiguous; MariaDB/MySQL report the
	// database name here, since there schema and database are the same thing.
	Schema     string `json:"schema"`
	Engine     string `json:"engine"`
	RowCount   int64  `json:"row_count"` // estimate for InnoDB
	DataBytes  int64  `json:"data_bytes"`
	IndexBytes int64  `json:"index_bytes"`
	Comment    string `json:"comment"`
}

// RowsPage is a page of table data. Values are stringified; nil = SQL NULL.
type RowsPage struct {
	Columns []string    `json:"columns"`
	Rows    [][]*string `json:"rows"`
	Total   int64       `json:"total"` // estimated total rows in the table
}

// ColumnInfo describes one column of a table.
type ColumnInfo struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Key      string  `json:"key"`     // PRI, UNI, MUL, or ""
	Default  *string `json:"default"` // nil = no default
	Extra    string  `json:"extra"`   // auto_increment, ...
	Comment  string  `json:"comment"`
}

// IndexInfo describes one index on a table.
type IndexInfo struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	Type    string   `json:"type"` // BTREE, FULLTEXT, ...
}

// TableSchema is a table's structure: columns, indexes and the CREATE DDL.
type TableSchema struct {
	Table   string       `json:"table"`
	Columns []ColumnInfo `json:"columns"`
	Indexes []IndexInfo  `json:"indexes"`
	DDL     string       `json:"ddl"`
}

// QueryResult is the outcome of a console query. For statements that return a
// result set, Columns/Rows are populated; for writes, RowsAffected is set.
type QueryResult struct {
	Columns      []string    `json:"columns"`
	Rows         [][]*string `json:"rows"`
	RowCount     int         `json:"row_count"`
	Truncated    bool        `json:"truncated"`     // more rows existed than the limit
	RowsAffected int64       `json:"rows_affected"` // for write statements
	ReadOnly     bool        `json:"read_only"`     // whether it ran as a read-only statement
	DurationMS   int64       `json:"duration_ms"`
}

// Admin is the set of live administration operations the dashboard uses
// (database users, grants, table browsing). Implemented per engine.
type Admin interface {
	ListDBUsers(ctx context.Context, p ConnParams) ([]DBUser, error)
	CreateDBUser(ctx context.Context, p ConnParams, user, host, password string) error
	DropDBUser(ctx context.Context, p ConnParams, user, host string) error
	// UserGrants returns the raw SHOW GRANTS statements for an account.
	UserGrants(ctx context.Context, p ConnParams, user, host string) ([]string, error)
	// SchemaGrants returns per-account privileges on one schema.
	SchemaGrants(ctx context.Context, p ConnParams, database string) ([]SchemaGrant, error)
	Grant(ctx context.Context, p ConnParams, user, host, database string, privileges []string) error
	Revoke(ctx context.Context, p ConnParams, user, host, database string) error

	ListTables(ctx context.Context, p ConnParams, database string) ([]TableInfo, error)
	TableRows(ctx context.Context, p ConnParams, database, table string, limit, offset int) (*RowsPage, error)

	// TableSchema returns a table's columns, indexes and CREATE DDL.
	TableSchema(ctx context.Context, p ConnParams, database, table string) (*TableSchema, error)
	// Query runs a single ad-hoc statement. Read-only statements return a
	// (capped) result set; writes are permitted only when allowWrite is true
	// and return the affected-row count.
	Query(ctx context.Context, p ConnParams, database, sql string, limit int, allowWrite bool) (*QueryResult, error)
	// ExportCSV streams a whole table (when table is set) or a read-only query
	// (when query is set) to w as CSV. onStart, if non-nil, is invoked once the
	// result set has opened successfully and before any bytes are written, so
	// callers can defer setting response headers to the success path. It returns
	// the number of data rows written.
	ExportCSV(ctx context.Context, p ConnParams, database, table, query string, w io.Writer, onStart func()) (int64, error)
}

// AdminFor returns the admin surface for the engine name.
func AdminFor(name string) (Admin, error) {
	c, err := For(name)
	if err != nil {
		return nil, err
	}
	a, ok := c.(Admin)
	if !ok {
		return nil, errNoAdmin(name)
	}
	return a, nil
}

type errNoAdmin string

func (e errNoAdmin) Error() string {
	return "engine: " + string(e) + " does not support live administration"
}

// GrantablePrivileges is the allowlist for schema-level grants.
var GrantablePrivileges = []string{
	"ALL PRIVILEGES", "SELECT", "INSERT", "UPDATE", "DELETE",
	"CREATE", "DROP", "ALTER", "INDEX", "REFERENCES",
	"CREATE VIEW", "SHOW VIEW", "TRIGGER", "EXECUTE",
	"CREATE ROUTINE", "ALTER ROUTINE", "EVENT", "LOCK TABLES",
	"CREATE TEMPORARY TABLES",
}
