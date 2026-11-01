package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// RowReader reads rows by key: page by page in key order, or by a list of
// keys. It backs comparing a table with a restored backup copy. Sessions
// are pinned to canonical settings (UTC, ISO dates) so two reads of the same
// value — from the live database and from the copy — produce the same text.
type RowReader interface {
	// ScanRows returns up to limit rows (full values) whose key is greater
	// than after (nil = from the start), in key order.
	ScanRows(ctx context.Context, p ConnParams, database, table string, after RowValues, limit int) (*ScanPage, error)
	// GetRows returns the rows with these keys (at most MaxGetRows); keys
	// with no row are simply absent.
	GetRows(ctx context.Context, p ConnParams, database, table string, keys []RowValues) (*ScanPage, error)
}

// MaxGetRows caps one GetRows call.
const MaxGetRows = 500

// ScanPage is a page of rows.
type ScanPage struct {
	Columns []string    `json:"columns"`
	Key     []string    `json:"key"`
	Rows    []RowValues `json:"rows"`
	// Next is the key to continue after; nil when the table is exhausted.
	Next RowValues `json:"next,omitempty"`
}

// CanonicalSession is the session the RowReader reads under, per engine;
// pass it to ApplyRowImage when writing images read by a RowReader.
func CanonicalSession(engineName string) map[string]string {
	if engineName == "postgres" {
		return map[string]string{"TimeZone": "UTC", "DateStyle": "ISO, MDY", "IntervalStyle": "postgres"}
	}
	return map[string]string{"time_zone": "+00:00"}
}

// KeyString is a stable text form of a row key for matching rows.
func KeyString(key []string, row RowValues) string {
	var b strings.Builder
	for _, k := range key {
		v := row[k]
		if v == nil {
			b.WriteString("\x01")
		} else {
			b.WriteString("\x02")
			b.WriteString(*v)
		}
		b.WriteString("\x00")
	}
	return b.String()
}

// RowHash fingerprints a row's values in cols (order-independent).
func RowHash(row RowValues, cols []string) string {
	sorted := append([]string(nil), cols...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, c := range sorted {
		h.Write([]byte(c))
		v, ok := row[c]
		switch {
		case !ok:
			h.Write([]byte{0})
		case v == nil:
			h.Write([]byte{1})
		default:
			h.Write([]byte{2})
			h.Write([]byte(*v))
		}
		h.Write([]byte{0xff})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// keyList renders "(k1, k2)" for the row key.
func (d dialect) keyList(m *tableMeta) string {
	names := make([]string, len(m.key))
	for i, k := range m.key {
		names[i] = d.quote(k)
	}
	return "(" + strings.Join(names, ", ") + ")"
}

// keyTuple renders "(?, ?)" binding key's values in key order.
func (d dialect) keyTuple(m *tableMeta, key RowValues, args []any) (string, []any, error) {
	parts := make([]string, len(m.key))
	for i, k := range m.key {
		v, ok := key[k]
		if !ok || v == nil {
			return "", nil, fmt.Errorf("the row key must be exactly: %s", strings.Join(m.key, ", "))
		}
		args = append(args, *v)
		parts[i] = d.param(len(args), m.byName[k])
	}
	return "(" + strings.Join(parts, ", ") + ")", args, nil
}

func (d dialect) columnList(m *tableMeta) string {
	names := make([]string, len(m.columns))
	for i, c := range m.columns {
		names[i] = d.quote(c.Name)
	}
	return strings.Join(names, ", ")
}

// scanSQL selects the next page in key order after a key.
func (d dialect) scanSQL(m *tableMeta, after RowValues, limit int) (string, []any, error) {
	if len(m.key) == 0 {
		return "", nil, ErrNoRowKey
	}
	q := fmt.Sprintf("SELECT %s FROM %s", d.columnList(m), m.qualified)
	var args []any
	if after != nil {
		tuple, a, err := d.keyTuple(m, after, nil)
		if err != nil {
			return "", nil, err
		}
		q += " WHERE " + d.keyList(m) + " > " + tuple
		args = a
	}
	order := make([]string, len(m.key))
	for i, k := range m.key {
		order[i] = d.quote(k)
	}
	return fmt.Sprintf("%s ORDER BY %s LIMIT %d", q, strings.Join(order, ", "), limit), args, nil
}

// getSQL selects the rows with any of keys.
func (d dialect) getSQL(m *tableMeta, keys []RowValues) (string, []any, error) {
	if len(m.key) == 0 {
		return "", nil, ErrNoRowKey
	}
	if len(keys) > MaxGetRows {
		return "", nil, fmt.Errorf("at most %d keys at a time", MaxGetRows)
	}
	if len(keys) == 0 {
		return fmt.Sprintf("SELECT %s FROM %s WHERE 1 = 0", d.columnList(m), m.qualified), nil, nil
	}
	var args []any
	tuples := make([]string, len(keys))
	for i, k := range keys {
		t, a, err := d.keyTuple(m, k, args)
		if err != nil {
			return "", nil, err
		}
		tuples[i], args = t, a
	}
	return fmt.Sprintf("SELECT %s FROM %s WHERE %s IN (%s)", d.columnList(m), m.qualified, d.keyList(m),
		strings.Join(tuples, ", ")), args, nil
}

// page wraps rows into a ScanPage; full means the scan may continue.
func (m *tableMeta) page(rows []RowValues, full bool) *ScanPage {
	p := &ScanPage{Key: m.key, Rows: rows}
	for _, c := range m.columns {
		p.Columns = append(p.Columns, c.Name)
	}
	if p.Rows == nil {
		p.Rows = []RowValues{}
	}
	if full && len(rows) > 0 {
		p.Next = m.keyOf(rows[len(rows)-1])
	}
	return p
}
