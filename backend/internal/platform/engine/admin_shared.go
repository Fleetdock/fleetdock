package engine

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// readOnlyLeadingKeywords are statement verbs treated as read-only. Anything
// else requires write permission. Even a misclassified read is still run inside
// a READ ONLY transaction where the engine supports it.
var readOnlyLeadingKeywords = map[string]bool{
	"SELECT": true, "SHOW": true, "DESCRIBE": true, "DESC": true,
	"EXPLAIN": true, "WITH": true, "TABLE": true,
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

// IsReadOnly reports whether a console statement is classified as a read.
func IsReadOnly(sqlText string) bool { return isReadOnlyStmt(sqlText) }

func isReadOnlyStmt(sqlText string) bool {
	kw := leadingKeyword(sqlText)
	if !readOnlyLeadingKeywords[kw] {
		return false
	}
	// EXPLAIN ANALYZE (PostgreSQL, MySQL 8) and ANALYZE FORMAT (MariaDB)
	// execute the statement they describe, so they are only as read-only as
	// that statement. Treat them as writes.
	if kw == "EXPLAIN" || kw == "DESC" || kw == "DESCRIBE" {
		if containsWord(strings.ToUpper(sqlText), "ANALYZE") {
			return false
		}
	}
	return true
}

// containsWord reports whether upper contains w as a whole word.
func containsWord(upper, w string) bool {
	for i := 0; ; {
		j := strings.Index(upper[i:], w)
		if j < 0 {
			return false
		}
		j += i
		before := j == 0 || !isWordByte(upper[j-1])
		after := j+len(w) == len(upper) || !isWordByte(upper[j+len(w)])
		if before && after {
			return true
		}
		i = j + len(w)
	}
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

// maxResultBytes caps the total cell bytes a console result returns, so a
// wide SELECT cannot balloon the API's memory or the browser tab.
const maxResultBytes = 8 << 20

// rowBytes is the size a stringified row contributes to maxResultBytes.
func rowBytes(row []*string) int {
	n := 0
	for _, c := range row {
		if c != nil {
			n += len(*c)
		}
	}
	return n
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

// BatchError reports which statement of a console batch failed.
type BatchError struct {
	Index int
	Err   error
}

func (e *BatchError) Error() string {
	return fmt.Sprintf("statement %d: %v", e.Index+1, e.Err)
}

func (e *BatchError) Unwrap() error { return e.Err }

// maxBatchStatements bounds one console run.
const maxBatchStatements = 100

// checkBatch validates a console batch before anything runs: no empty batch,
// and no write statement unless writes are allowed — refused up front, so a
// batch never half-runs and then fails on permission.
func checkBatch(stmts []string, allowWrite bool) error {
	if len(stmts) == 0 {
		return fmt.Errorf("query is empty")
	}
	if len(stmts) > maxBatchStatements {
		return fmt.Errorf("too many statements in one run (max %d)", maxBatchStatements)
	}
	for i, s := range stmts {
		s = strings.TrimSpace(s)
		if s == "" {
			return fmt.Errorf("statement %d is empty", i+1)
		}
		if !allowWrite && !isReadOnlyStmt(s) {
			return fmt.Errorf("statement %d looks like a write statement; running it requires database:write", i+1)
		}
	}
	return nil
}

func clampQueryLimit(limit int) int {
	if limit <= 0 || limit > 1000 {
		return 100
	}
	return limit
}

func singleResult(rs []QueryResult, err error) (*QueryResult, error) {
	if err != nil {
		var be *BatchError
		if errors.As(err, &be) {
			return nil, be.Err
		}
		return nil, err
	}
	return &rs[0], nil
}
