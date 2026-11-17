package engine

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Browsing builds on the shared dialect: values are typed query parameters
// ({p1:Int64}), substituted by the server, never spliced into the SQL. The
// write half of DataEditor is refused: ClickHouse has no row keys and its
// UPDATE/DELETE are asynchronous mutations, which cannot honour the data
// browser's one-row-exactly, undoable contract.

var clickhouseDialect = dialect{
	quote: quoteCHIdent,
	param: func(n int, col column) string {
		t := col.Type
		if t == "" || t == "text" {
			t = "String"
		}
		return fmt.Sprintf("{p%d:%s}", n, t)
	},
	text: func(q string) string { return "toString(" + q + ")" },
	like: "ILIKE",
	concat: func(parts ...string) string {
		return "concat(" + strings.Join(parts, ", ") + ")"
	},
}

// chParams binds dialect arguments to the p1..pN placeholders. Parameter
// values use the server's escaped text form, so backslashes, tabs and
// newlines are escaped.
func chParams(args []any) map[string]string {
	esc := strings.NewReplacer(`\`, `\\`, "\t", `\t`, "\n", `\n`, "\r", `\r`)
	out := make(map[string]string, len(args))
	for i, a := range args {
		var s string
		switch v := a.(type) {
		case string:
			s = v
		case *string:
			if v == nil {
				s = `\N`
				out["p"+strconv.Itoa(i+1)] = s
				continue
			}
			s = *v
		default:
			s = fmt.Sprint(v)
		}
		out["p"+strconv.Itoa(i+1)] = esc.Replace(s)
	}
	return out
}

// chMeta loads a table's columns. The key it returns is the table's sorting
// key (its plain-column prefix), used only to make paging stable: ClickHouse
// does not enforce uniqueness, so no key addresses a single row.
func (c *ClickHouse) chMeta(ctx context.Context, p ConnParams, database, table string) (*tableMeta, error) {
	qt, err := chQualified(database, table)
	if err != nil {
		return nil, err
	}
	params := map[string]string{"db": database, "t": table}
	rows, err := c.rows(ctx, p, chRequest{
		query: `
			SELECT name, type, default_kind
			FROM system.columns WHERE database = {db:String} AND table = {t:String}
			ORDER BY position`,
		params: params,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("table %q not found", table)
	}
	cols := make([]column, 0, len(rows))
	for _, r := range rows {
		t := str(r[1])
		kind := str(r[2])
		cols = append(cols, column{
			Name:       str(r[0]),
			Type:       t,
			Nullable:   strings.HasPrefix(t, "Nullable(") || strings.HasPrefix(t, "LowCardinality(Nullable("),
			HasDefault: true, // every ClickHouse column has an implicit default
			Generated:  kind == "MATERIALIZED" || kind == "ALIAS",
		})
	}
	m := newTableMeta(qt, cols, nil)
	sk, err := c.rows(ctx, p, chRequest{
		query:  "SELECT sorting_key FROM system.tables WHERE database = {db:String} AND name = {t:String}",
		params: params,
	})
	if err == nil && len(sk) > 0 {
		for _, part := range strings.Split(str(sk[0][0]), ",") {
			name := strings.Trim(strings.TrimSpace(part), "`")
			if _, ok := m.byName[name]; !ok {
				break // an expression (toDate(ts), ...): stop at the plain-column prefix
			}
			m.key = append(m.key, name)
		}
	}
	return m, nil
}

// BrowseRows returns a filtered, sorted page of a table.
func (c *ClickHouse) BrowseRows(ctx context.Context, p ConnParams, database string, req BrowseRequest) (*BrowseResult, error) {
	clampBrowse(&req)
	meta, err := c.chMeta(ctx, p, database, req.Table)
	if err != nil {
		return nil, err
	}
	d := clickhouseDialect
	where, args, err := d.where(meta, req, nil)
	if err != nil {
		return nil, err
	}
	order, err := d.orderBy(meta, req.Sort)
	if err != nil {
		return nil, err
	}
	params := chParams(args)

	res := &BrowseResult{Columns: browseColumns(meta), Rows: [][]*string{}, Key: []string{}}
	if filtered(req) {
		rows, err := c.rows(ctx, p, chRequest{
			query:    fmt.Sprintf("SELECT count() FROM (SELECT 1 FROM %s%s LIMIT %d)", meta.qualified, where, TotalCap+1),
			database: database, params: params,
		})
		if err != nil {
			return nil, err
		}
		res.Total = i64(rows[0][0])
		res.TotalExact = true
		if res.Total > TotalCap {
			res.Total, res.TotalCapped = TotalCap, true
		}
	} else {
		res.Total, _ = c.estimatedRows(ctx, p, database, req.Table)
	}

	ctx, cancel := withDefaultTimeout(ctx, chTimeout)
	defer cancel()
	resp, err := c.open(ctx, p, chRequest{
		query:    fmt.Sprintf("SELECT * FROM %s%s%s LIMIT %d OFFSET %d", meta.qualified, where, order, req.Limit, req.Offset),
		database: database, params: params, readOnly: true,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Close()
	if _, _, err := resp.header(); err != nil {
		return nil, err
	}
	for {
		row, ok, err := resp.row(4096)
		if err != nil {
			return nil, err
		}
		if !ok {
			return res, nil
		}
		res.Rows = append(res.Rows, row)
	}
}

// ExportRowsCSV streams the rows a browse request selects to w as CSV.
func (c *ClickHouse) ExportRowsCSV(ctx context.Context, p ConnParams, database string, req BrowseRequest, w io.Writer, onStart func()) (int64, error) {
	meta, err := c.chMeta(ctx, p, database, req.Table)
	if err != nil {
		return 0, err
	}
	d := clickhouseDialect
	where, args, err := d.where(meta, req, nil)
	if err != nil {
		return 0, err
	}
	order, err := d.orderBy(meta, req.Sort)
	if err != nil {
		return 0, err
	}
	return c.streamCSV(ctx, p, chRequest{
		query:    "SELECT * FROM " + meta.qualified + where + order,
		database: database, params: chParams(args),
	}, w, onStart)
}

// InsertRow is refused: see ErrReadOnlyEngine.
func (c *ClickHouse) InsertRow(context.Context, ConnParams, string, string, RowValues) (*RowChange, error) {
	return nil, ErrReadOnlyEngine
}

// UpdateRow is refused: see ErrReadOnlyEngine.
func (c *ClickHouse) UpdateRow(context.Context, ConnParams, string, string, RowValues, RowValues) (*RowChange, error) {
	return nil, ErrReadOnlyEngine
}

// DeleteRow is refused: see ErrReadOnlyEngine.
func (c *ClickHouse) DeleteRow(context.Context, ConnParams, string, string, RowValues) (*RowChange, error) {
	return nil, ErrReadOnlyEngine
}

// ApplyRowImage is refused: see ErrReadOnlyEngine.
func (c *ClickHouse) ApplyRowImage(context.Context, ConnParams, string, string, ApplyRequest) (*RowChange, error) {
	return nil, ErrReadOnlyEngine
}

// ImportCSV is refused: see ErrReadOnlyEngine.
func (c *ClickHouse) ImportCSV(context.Context, ConnParams, string, string, io.Reader, ImportOptions) (int64, error) {
	return 0, ErrReadOnlyEngine
}
