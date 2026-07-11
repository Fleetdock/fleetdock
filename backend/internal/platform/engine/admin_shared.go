package engine

import (
	"database/sql"
	"fmt"
	"strings"
)

// readOnlyLeadingKeywords are statement verbs treated as read-only. Anything
// else requires write permission. Even a misclassified read is still run inside
// a READ ONLY transaction where the engine supports it.
var readOnlyLeadingKeywords = map[string]bool{
	"SELECT": true, "SHOW": true, "DESCRIBE": true, "DESC": true,
	"EXPLAIN": true, "WITH": true, "ANALYZE": true, "TABLE": true,
}

// leadingKeyword extracts the first SQL keyword, skipping leading whitespace,
// line (`-- `, `#`) and block (`/* */`) comments and opening parentheses.
func leadingKeyword(sqlText string) string {
	s := sqlText
	for {
		s = strings.TrimLeft(s, " \t\r\n(")
		switch {
		case strings.HasPrefix(s, "--"), strings.HasPrefix(s, "#"):
			if i := strings.IndexAny(s, "\r\n"); i >= 0 {
				s = s[i+1:]
				continue
			}
			return ""
		case strings.HasPrefix(s, "/*"):
			if i := strings.Index(s, "*/"); i >= 0 {
				s = s[i+2:]
				continue
			}
			return ""
		}
		break
	}
	i := 0
	for i < len(s) && isWordByte(s[i]) {
		i++
	}
	if i == 0 {
		return ""
	}
	return strings.ToUpper(s[:i])
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func isReadOnlyStmt(sqlText string) bool {
	return readOnlyLeadingKeywords[leadingKeyword(sqlText)]
}

// validTableName rejects identifiers that could break quoting.
func validTableName(table string) bool {
	return table != "" && len(table) <= 64 && !strings.ContainsAny(table, "`'\"\\\n\r\x00")
}

// scanStringRow scans the current row into stringified values, truncating each
// value to maxLen bytes (maxLen <= 0 disables truncation). nil = SQL NULL.
func scanStringRow(rows *sql.Rows, n, maxLen int) ([]*string, error) {
	raw := make([]sql.RawBytes, n)
	ptrs := make([]any, n)
	for i := range raw {
		ptrs[i] = &raw[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	row := make([]*string, n)
	for i, b := range raw {
		if b == nil {
			continue
		}
		s := string(b)
		if maxLen > 0 && len(s) > maxLen {
			s = s[:maxLen] + "…"
		}
		row[i] = &s
	}
	return row, nil
}

// maxExportRows caps a CSV export so a runaway query cannot stream forever.
const maxExportRows = 1_000_000

// stringifyCell converts a scanned driver value to a display string.
func stringifyCell(v any, maxLen int) *string {
	if v == nil {
		return nil
	}
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case []byte:
		s = string(x)
	default:
		s = fmt.Sprintf("%v", v)
	}
	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen] + "…"
	}
	return &s
}
