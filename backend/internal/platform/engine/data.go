package engine

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Data editing: filtered browsing and single-row insert/update/delete, plus
// CSV import. Shared by both engine families through a small dialect; the
// engine-specific parts are fetching table metadata and running statements.
//
// Safety rules, enforced here rather than trusted to callers:
//   - every column name is checked against the table's real columns and then
//     quoted; nothing user-supplied is spliced into SQL unquoted;
//   - every value is a bind parameter;
//   - a row is addressed by its primary key (or a NOT NULL unique key), and a
//     change must hit exactly one row or it is rolled back.

// FilterOp is a comparison allowed in a browse filter.
type FilterOp string

const (
	OpEq         FilterOp = "eq"
	OpNe         FilterOp = "ne"
	OpLt         FilterOp = "lt"
	OpLte        FilterOp = "lte"
	OpGt         FilterOp = "gt"
	OpGte        FilterOp = "gte"
	OpContains   FilterOp = "contains"
	OpStartsWith FilterOp = "starts_with"
	OpIsNull     FilterOp = "is_null"
	OpNotNull    FilterOp = "not_null"
)

// Filter restricts browsed rows by one column.
type Filter struct {
	Column string   `json:"column"`
	Op     FilterOp `json:"op"`
	Value  *string  `json:"value"`
}

// SortKey orders browsed rows by one column.
type SortKey struct {
	Column string `json:"column"`
	Desc   bool   `json:"desc"`
}

// BrowseRequest selects a page of rows.
type BrowseRequest struct {
	Table   string    `json:"-"`
	Filters []Filter  `json:"filters"`
	Sort    []SortKey `json:"sort"`
	// Search matches any column's text representation (case-insensitive).
	Search string `json:"search"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// BrowseResult is a page of rows plus what the dashboard needs to edit them.
type BrowseResult struct {
	Columns []BrowseColumn `json:"columns"`
	Rows    [][]*string    `json:"rows"`
	// Total counts matching rows: an estimate without filters, exact (up to
	// TotalCap) with them.
	Total       int64 `json:"total"`
	TotalExact  bool  `json:"total_exact"`
	TotalCapped bool  `json:"total_capped"`
	// Key names the columns that identify a row; empty means the table has
	// no primary or NOT NULL unique key and rows cannot be edited.
	Key []string `json:"key"`
}

// BrowseColumn describes a result column.
type BrowseColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	// HasDefault means the column may be omitted on insert.
	HasDefault bool `json:"has_default"`
}

// RowValues maps column name to value (nil = SQL NULL).
type RowValues map[string]*string

// ImportOptions control a CSV import.
type ImportOptions struct {
	// EmptyAsNull loads empty fields as NULL instead of empty strings. A
	// field of exactly \N is always NULL.
	EmptyAsNull bool
}

// DataEditor is the data-editing surface of an engine.
type DataEditor interface {
	BrowseRows(ctx context.Context, p ConnParams, database string, req BrowseRequest) (*BrowseResult, error)
	InsertRow(ctx context.Context, p ConnParams, database, table string, values RowValues) error
	UpdateRow(ctx context.Context, p ConnParams, database, table string, key, values RowValues) error
	DeleteRow(ctx context.Context, p ConnParams, database, table string, key RowValues) error
	ImportCSV(ctx context.Context, p ConnParams, database, table string, r io.Reader, opts ImportOptions) (int64, error)
}

// DataEditorFor returns the data-editing surface for the engine name.
func DataEditorFor(name string) (DataEditor, error) {
	c, err := For(name)
	if err != nil {
		return nil, err
	}
	d, ok := c.(DataEditor)
	if !ok {
		return nil, fmt.Errorf("engine: %s does not support data editing", name)
	}
	return d, nil
}

// ErrNoRowKey is returned when editing a table that has no way to address a
// single row.
var ErrNoRowKey = errors.New("this table has no primary key or NOT NULL unique key, so rows cannot be edited individually; use the SQL console")

const (
	maxBrowseLimit = 500
	// TotalCap bounds the exact count of filtered rows.
	TotalCap = 10000
	// importBatchParams keeps a multi-row INSERT under both engines'
	// placeholder limits (MySQL 65535, PostgreSQL 65535).
	importBatchParams = 30000
	maxImportBatch    = 500
)

// column is a column's metadata as the builder needs it.
type column struct {
	Name string
	// Type is the engine's full type (varchar(20), numeric(10,2), ...),
	// taken from the catalog, used for PostgreSQL parameter casts.
	Type       string
	Nullable   bool
	HasDefault bool
}

// tableMeta is a table's columns and row key.
type tableMeta struct {
	qualified string // quoted, fully qualified table name
	columns   []column
	byName    map[string]column
	key       []string
}

func newTableMeta(qualified string, cols []column, key []string) *tableMeta {
	m := &tableMeta{qualified: qualified, columns: cols, byName: make(map[string]column, len(cols)), key: key}
	for _, c := range cols {
		m.byName[c.Name] = c
	}
	return m
}

// chooseKey picks the primary key, else the first unique key whose columns
// are all NOT NULL. candidates must list the primary key first if present.
func chooseKey(candidates [][]string, cols map[string]column) []string {
	for _, k := range candidates {
		ok := len(k) > 0
		for _, c := range k {
			if col, found := cols[c]; !found || col.Nullable {
				ok = false
				break
			}
		}
		if ok {
			return k
		}
	}
	return nil
}

// dialect is what differs between the engines' SQL.
type dialect struct {
	quote func(string) string
	// param renders the n-th (1-based) bind parameter for a value of col.
	param func(n int, col column) string
	// text renders a column as text for search/contains.
	text func(quoted string) string
	// like is the case-insensitive LIKE operator.
	like string
	// concat renders a string concatenation of SQL expressions.
	concat func(parts ...string) string
	// positional means placeholders are anonymous (MySQL "?"): a value used
	// in several places needs one argument per use.
	positional bool
}

var opSQL = map[FilterOp]string{OpEq: "=", OpNe: "<>", OpLt: "<", OpLte: "<=", OpGt: ">", OpGte: ">="}

// where builds the WHERE clause and its arguments for a browse request.
func (d dialect) where(m *tableMeta, req BrowseRequest, args []any) (string, []any, error) {
	var conds []string
	for _, f := range req.Filters {
		col, ok := m.byName[f.Column]
		if !ok {
			return "", nil, fmt.Errorf("unknown column %q", f.Column)
		}
		q := d.quote(col.Name)
		switch f.Op {
		case OpIsNull:
			conds = append(conds, q+" IS NULL")
		case OpNotNull:
			conds = append(conds, q+" IS NOT NULL")
		case OpContains, OpStartsWith:
			if f.Value == nil {
				return "", nil, fmt.Errorf("filter on %q needs a value", f.Column)
			}
			args = append(args, escapeLike(*f.Value))
			pattern := d.concat(d.param(len(args), column{Type: "text"}), "'%'")
			if f.Op == OpContains {
				pattern = d.concat("'%'", d.param(len(args), column{Type: "text"}), "'%'")
			}
			conds = append(conds, fmt.Sprintf("%s %s %s", d.text(q), d.like, pattern))
		default:
			op, ok := opSQL[f.Op]
			if !ok {
				return "", nil, fmt.Errorf("unsupported filter operator %q", f.Op)
			}
			if f.Value == nil {
				return "", nil, fmt.Errorf("filter on %q needs a value (use is_null for NULL)", f.Column)
			}
			args = append(args, *f.Value)
			conds = append(conds, fmt.Sprintf("%s %s %s", q, op, d.param(len(args), col)))
		}
	}
	if s := strings.TrimSpace(req.Search); s != "" {
		term := escapeLike(s)
		args = append(args, term)
		n := len(args)
		var ors []string
		for i, c := range m.columns {
			if d.positional && i > 0 {
				args = append(args, term)
				n = len(args)
			}
			pattern := d.concat("'%'", d.param(n, column{Type: "text"}), "'%'")
			ors = append(ors, fmt.Sprintf("%s %s %s", d.text(d.quote(c.Name)), d.like, pattern))
		}
		if len(ors) > 0 {
			conds = append(conds, "("+strings.Join(ors, " OR ")+")")
		}
	}
	if len(conds) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args, nil
}

// orderBy renders the requested sort, always followed by the row key so pages
// are stable (no row skipped or repeated between pages).
func (d dialect) orderBy(m *tableMeta, sort []SortKey) (string, error) {
	var parts []string
	seen := map[string]bool{}
	for _, s := range sort {
		if _, ok := m.byName[s.Column]; !ok {
			return "", fmt.Errorf("unknown sort column %q", s.Column)
		}
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		parts = append(parts, d.quote(s.Column)+" "+dir)
		seen[s.Column] = true
	}
	for _, k := range m.key {
		if !seen[k] {
			parts = append(parts, d.quote(k)+" ASC")
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " ORDER BY " + strings.Join(parts, ", "), nil
}

// keyWhere renders "k1 = ? AND k2 = ?" for a row key.
func (d dialect) keyWhere(m *tableMeta, key RowValues, args []any) (string, []any, error) {
	if len(m.key) == 0 {
		return "", nil, ErrNoRowKey
	}
	if len(key) != len(m.key) {
		return "", nil, fmt.Errorf("the row key must be exactly: %s", strings.Join(m.key, ", "))
	}
	conds := make([]string, 0, len(m.key))
	for _, k := range m.key {
		v, ok := key[k]
		if !ok || v == nil {
			return "", nil, fmt.Errorf("the row key must be exactly: %s", strings.Join(m.key, ", "))
		}
		args = append(args, *v)
		conds = append(conds, fmt.Sprintf("%s = %s", d.quote(k), d.param(len(args), m.byName[k])))
	}
	return strings.Join(conds, " AND "), args, nil
}

// sortedColumns validates value columns and returns them in table order, so
// generated SQL is deterministic.
func (m *tableMeta) sortedColumns(values RowValues) ([]column, error) {
	for name := range values {
		if _, ok := m.byName[name]; !ok {
			return nil, fmt.Errorf("unknown column %q", name)
		}
	}
	out := make([]column, 0, len(values))
	for _, c := range m.columns {
		if _, ok := values[c.Name]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func (d dialect) insertSQL(m *tableMeta, values RowValues) (string, []any, error) {
	cols, err := m.sortedColumns(values)
	if err != nil {
		return "", nil, err
	}
	if len(cols) == 0 {
		return "", nil, fmt.Errorf("no values to insert")
	}
	names := make([]string, len(cols))
	params := make([]string, len(cols))
	args := make([]any, len(cols))
	for i, c := range cols {
		names[i] = d.quote(c.Name)
		args[i] = values[c.Name]
		params[i] = d.param(i+1, c)
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", m.qualified,
		strings.Join(names, ", "), strings.Join(params, ", ")), args, nil
}

func (d dialect) updateSQL(m *tableMeta, key, values RowValues) (string, []any, error) {
	cols, err := m.sortedColumns(values)
	if err != nil {
		return "", nil, err
	}
	if len(cols) == 0 {
		return "", nil, fmt.Errorf("no values to update")
	}
	sets := make([]string, len(cols))
	args := make([]any, 0, len(cols)+len(key))
	for i, c := range cols {
		args = append(args, values[c.Name])
		sets[i] = fmt.Sprintf("%s = %s", d.quote(c.Name), d.param(len(args), c))
	}
	where, args, err := d.keyWhere(m, key, args)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("UPDATE %s SET %s WHERE %s", m.qualified, strings.Join(sets, ", "), where), args, nil
}

func (d dialect) deleteSQL(m *tableMeta, key RowValues) (string, []any, error) {
	where, args, err := d.keyWhere(m, key, nil)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("DELETE FROM %s WHERE %s", m.qualified, where), args, nil
}

// batchInsertSQL renders one multi-row INSERT for len(rows) rows of cols.
func (d dialect) batchInsertSQL(m *tableMeta, cols []column, rows [][]*string) (string, []any) {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = d.quote(c.Name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "INSERT INTO %s (%s) VALUES ", m.qualified, strings.Join(names, ", "))
	args := make([]any, 0, len(cols)*len(rows))
	for r, row := range rows {
		if r > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('(')
		for i, c := range cols {
			if i > 0 {
				b.WriteString(", ")
			}
			args = append(args, row[i])
			b.WriteString(d.param(len(args), c))
		}
		b.WriteByte(')')
	}
	return b.String(), args
}

// exactlyOne turns an affected-row count into the editing contract.
func exactlyOne(n int64) error {
	switch n {
	case 1:
		return nil
	case 0:
		return fmt.Errorf("no row matched; it may have been changed or deleted meanwhile — reload and try again")
	default:
		return fmt.Errorf("the key matched %d rows; nothing was changed", n)
	}
}

// csvBatches reads a CSV whose header names table columns and calls insert
// for each batch of rows. It returns the number of rows read.
func csvBatches(m *tableMeta, r io.Reader, opts ImportOptions, insert func(cols []column, rows [][]*string) error) (int64, error) {
	cr := csv.NewReader(r)
	cr.ReuseRecord = false
	header, err := cr.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return 0, fmt.Errorf("the CSV file is empty")
		}
		return 0, fmt.Errorf("read CSV header: %w", err)
	}
	cols := make([]column, len(header))
	seen := map[string]bool{}
	for i, h := range header {
		h = strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF"))
		c, ok := m.byName[h]
		if !ok {
			return 0, fmt.Errorf("CSV column %q does not exist in the table", h)
		}
		if seen[h] {
			return 0, fmt.Errorf("CSV column %q appears twice", h)
		}
		seen[h] = true
		cols[i] = c
	}
	batch := importBatchParams / len(cols)
	if batch > maxImportBatch {
		batch = maxImportBatch
	}
	if batch < 1 {
		batch = 1
	}

	var total int64
	rows := make([][]*string, 0, batch)
	line := 1
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line++
		if err != nil {
			return total, fmt.Errorf("CSV line %d: %w", line, err)
		}
		if len(rec) != len(cols) {
			return total, fmt.Errorf("CSV line %d has %d fields, the header has %d", line, len(rec), len(cols))
		}
		row := make([]*string, len(rec))
		for i, v := range rec {
			if v == `\N` || (opts.EmptyAsNull && v == "") {
				continue
			}
			v := v
			row[i] = &v
		}
		rows = append(rows, row)
		if len(rows) == batch {
			if err := insert(cols, rows); err != nil {
				return total, fmt.Errorf("CSV rows ending at line %d: %w", line, err)
			}
			total += int64(len(rows))
			rows = rows[:0]
		}
	}
	if len(rows) > 0 {
		if err := insert(cols, rows); err != nil {
			return total, fmt.Errorf("CSV rows ending at line %d: %w", line, err)
		}
		total += int64(len(rows))
	}
	return total, nil
}

// escapeLike escapes LIKE wildcards so a search for "50%" matches literally.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func clampBrowse(req *BrowseRequest) {
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > maxBrowseLimit {
		req.Limit = maxBrowseLimit
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
}

func browseColumns(m *tableMeta) []BrowseColumn {
	out := make([]BrowseColumn, len(m.columns))
	for i, c := range m.columns {
		out[i] = BrowseColumn(c)
	}
	return out
}

func filtered(req BrowseRequest) bool {
	return len(req.Filters) > 0 || strings.TrimSpace(req.Search) != ""
}
