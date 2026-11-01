package engine

import (
	"context"
	"fmt"
	"strings"
)

// Preview (dry run): a console script with writes runs inside one
// transaction that is always rolled back, reporting how many rows each
// statement would change and, for simple single-table UPDATE / DELETE, a
// sample of the rows before and after.

// Previewer is implemented by engines that can dry-run a script.
type Previewer interface {
	// PreviewBatch runs stmts in one transaction, then rolls it back. Every
	// statement must pass PreviewBlocker.
	PreviewBatch(ctx context.Context, p ConnParams, database string, stmts []string, limit int, onSession func(id int64)) ([]QueryResult, error)
}

// PreviewSample is a few of the rows a statement touched, as they were
// before it and as it left them.
type PreviewSample struct {
	Table   string       `json:"table"`
	Key     []string     `json:"key"`
	Columns []string     `json:"columns"`
	Rows    []PreviewRow `json:"rows"`
	// Truncated: the statement touched more rows than the sample shows.
	Truncated bool `json:"truncated"`
}

// PreviewRow is one sampled row. After is nil when the statement deleted it.
type PreviewRow struct {
	Before []*string `json:"before"`
	After  []*string `json:"after"`
}

const (
	previewSampleRows = 20
	previewCellBytes  = 1024
)

// previewAllowed are the statements a preview runs: reads and DML, all of
// which a transaction rollback undoes. Everything else (DDL, which MySQL
// commits implicitly; transaction control; SET; CALL; LOCK; LOAD…) is
// refused so the rollback really undoes the run.
var previewAllowed = map[string]bool{
	"SELECT": true, "WITH": true, "TABLE": true, "VALUES": true, "SHOW": true,
	"DESCRIBE": true, "DESC": true, "EXPLAIN": true,
	"INSERT": true, "UPDATE": true, "DELETE": true, "REPLACE": true, "MERGE": true,
}

// PreviewBlocker returns why a statement cannot be previewed, or nil.
func PreviewBlocker(stmt string) error {
	kw := leadingKeyword(stmt)
	if !previewAllowed[kw] {
		return fmt.Errorf("%s statements cannot be previewed: only SELECT, INSERT, UPDATE, DELETE, REPLACE and MERGE can be "+
			"rolled back reliably", kw)
	}
	words := topLevelWords(stmt)
	for i, w := range words {
		if w.upper == "INTO" && i+1 < len(words) && (words[i+1].upper == "OUTFILE" || words[i+1].upper == "DUMPFILE") {
			return fmt.Errorf("SELECT … INTO %s writes a file, which a rollback cannot undo", words[i+1].upper)
		}
	}
	return nil
}

// word is a top-level keyword or identifier of a statement, with its byte
// offsets.
type word struct {
	upper      string
	start, end int
}

// topLevelWords returns the bare words of stmt outside quotes, comments,
// dollar quotes and parentheses, plus "," and "." as single-byte words.
func topLevelWords(s string) []word {
	var out []word
	depth := 0
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			j := skipQuoted(s, i, c)
			if depth == 0 && c != '\'' {
				// A quoted identifier is a word.
				out = append(out, word{upper: s[i:j], start: i, end: j})
			}
			i = j
			continue
		case c == '-' && strings.HasPrefix(s[i:], "--"), c == '#':
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				return out
			}
			i += j
			continue
		case c == '/' && strings.HasPrefix(s[i:], "/*"):
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				return out
			}
			i += j + 4
			continue
		case c == '$':
			if tag, ok := dollarTag(s[i:]); ok {
				end := strings.Index(s[i+len(tag):], tag)
				if end < 0 {
					return out
				}
				i += len(tag) + end + len(tag)
				continue
			}
		case c == '(':
			depth++
		case c == ')':
			depth--
		case (c == ',' || c == '.') && depth == 0:
			out = append(out, word{upper: string(c), start: i, end: i + 1})
		case isWordByte(c) || c == '$':
			j := i
			for j < len(s) && (isWordByte(s[j]) || s[j] == '$') {
				j++
			}
			if depth == 0 {
				out = append(out, word{upper: strings.ToUpper(s[i:j]), start: i, end: j})
			}
			i = j
			continue
		}
		i++
	}
	return out
}

// dmlTarget is what parseSingleTableDML found.
type dmlTarget struct {
	op    string   // UPDATE or DELETE
	table string   // the table as written ("a", `db`.`t`, "s"."t")
	parts []string // the table name, unquoted, split on "."
	alias string
	cond  string // the WHERE condition; "" = every row
}

var dmlModifiers = map[string]bool{"LOW_PRIORITY": true, "IGNORE": true, "QUICK": true, "ONLY": true}

// parseSingleTableDML recognises "UPDATE t [AS a] SET … [WHERE …]" and
// "DELETE FROM t [AS a] [WHERE …]" (optionally ending in RETURNING) — the
// shapes whose touched rows "SELECT * FROM t [a] WHERE …" finds. Joins,
// several tables, USING / FROM clauses, ORDER BY and LIMIT are not
// recognised (ok=false): the preview then reports counts only.
func parseSingleTableDML(stmt string) (dmlTarget, bool) {
	ws := topLevelWords(stmt)
	var t dmlTarget
	if len(ws) < 2 {
		return t, false
	}
	i := 1
	switch ws[0].upper {
	case "UPDATE":
		t.op = "UPDATE"
	case "DELETE":
		t.op = "DELETE"
		for i < len(ws) && dmlModifiers[ws[i].upper] {
			i++
		}
		if i >= len(ws) || ws[i].upper != "FROM" {
			return t, false // multi-table DELETE t1 FROM …
		}
		i++
	default:
		return t, false
	}
	for i < len(ws) && dmlModifiers[ws[i].upper] {
		i++
	}
	// Table name: word ("." word)*.
	if i >= len(ws) || ws[i].upper == "," || ws[i].upper == "." {
		return t, false
	}
	start := ws[i].start
	end := ws[i].end
	t.parts = []string{unquoteIdent(stmt[ws[i].start:ws[i].end])}
	i++
	for i+1 < len(ws) && ws[i].upper == "." && ws[i].start == end {
		end = ws[i+1].end
		t.parts = append(t.parts, unquoteIdent(stmt[ws[i+1].start:ws[i+1].end]))
		i += 2
	}
	t.table = stmt[start:end]
	if len(t.parts) > 2 {
		return t, false
	}
	// Optional alias.
	if i < len(ws) && ws[i].upper == "AS" {
		i++
		if i >= len(ws) {
			return t, false
		}
		t.alias = stmt[ws[i].start:ws[i].end]
		i++
	} else if i < len(ws) && !isClauseWord(ws[i].upper) && ws[i].upper != "," {
		t.alias = stmt[ws[i].start:ws[i].end]
		i++
	}
	if t.op == "UPDATE" {
		if i >= len(ws) || ws[i].upper != "SET" {
			return t, false // a comma-separated list or a JOIN
		}
	}
	whereAt := -1
	condEnd := len(stmt)
	for j := i; j < len(ws); j++ {
		switch ws[j].upper {
		case "JOIN", "USING", "ORDER", "LIMIT":
			return t, false
		case "FROM":
			// UPDATE … SET … FROM other_table (PostgreSQL) joins.
			return t, false
		case "WHERE":
			if whereAt < 0 {
				whereAt = ws[j].end
			}
		case "RETURNING":
			if whereAt >= 0 || t.op == "DELETE" {
				condEnd = ws[j].start
			}
			j = len(ws)
		case ",":
			if t.op == "DELETE" {
				return t, false
			}
		}
	}
	if whereAt >= 0 {
		t.cond = strings.TrimSpace(stmt[whereAt:condEnd])
		if t.cond == "" {
			return t, false
		}
	}
	return t, true
}

func isClauseWord(w string) bool {
	switch w {
	case "SET", "WHERE", "RETURNING", "ORDER", "LIMIT", "JOIN", "INNER", "LEFT", "RIGHT", "CROSS", "NATURAL",
		"STRAIGHT_JOIN", "USING", "FROM", "PARTITION":
		return true
	}
	return false
}

// unquoteIdent strips one level of `…` or "…" quoting.
func unquoteIdent(s string) string {
	if len(s) >= 2 && (s[0] == '`' || s[0] == '"') && s[len(s)-1] == s[0] {
		q := string(s[0])
		return strings.ReplaceAll(s[1:len(s)-1], q+q, q)
	}
	return s
}

// sampleSQL selects the rows a parsed UPDATE / DELETE touches.
func (t dmlTarget) sampleSQL(limit int) string {
	from := t.table
	if t.alias != "" {
		from += " " + t.alias
	}
	q := "SELECT * FROM " + from
	if t.cond != "" {
		// On its own lines: a trailing "-- comment" in the condition must
		// not swallow the LIMIT.
		q += " WHERE (" + t.cond + "\n)"
	}
	return fmt.Sprintf("%s\nLIMIT %d", q, limit+1)
}

// previewNotes are true of every preview.
var previewNotes = []string{
	"Rolled back: nothing was saved.",
	"Triggers ran, and sequences / AUTO_INCREMENT counters advanced, as in a real run.",
	"The rows were locked while the preview ran, as a real run would lock them.",
}

// truncateCells shortens values for display.
func truncateCells(row []*string) []*string {
	out := make([]*string, len(row))
	for i, v := range row {
		out[i] = stringifyCell(derefOrNil(v), previewCellBytes)
	}
	return out
}

func derefOrNil(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

// newSample starts a sample for t; before is trimmed to the sample size.
func newSample(t dmlTarget, meta *tableMeta, before *[]RowValues) *PreviewSample {
	s := &PreviewSample{Table: strings.Join(t.parts, "."), Key: meta.key, Rows: []PreviewRow{}}
	for _, c := range meta.columns {
		s.Columns = append(s.Columns, c.Name)
	}
	if len(*before) > previewSampleRows {
		*before = (*before)[:previewSampleRows]
		s.Truncated = true
	}
	return s
}

// previewAfter fills a sample: each sampled row as it was, and as the
// statement left it (re-read by key; nil when it was deleted).
func previewAfter(_ context.Context, s *PreviewSample, meta *tableMeta, before []RowValues, read func(RowValues) (RowValues, error)) error {
	for _, b := range before {
		row := PreviewRow{Before: truncateCells(valuesIn(meta, b))}
		key := meta.keyOf(b)
		if key != nil {
			after, err := read(key)
			if err != nil {
				return err
			}
			if after != nil {
				row.After = truncateCells(valuesIn(meta, after))
			}
		}
		s.Rows = append(s.Rows, row)
	}
	return nil
}

// valuesIn orders a row's values like meta's columns.
func valuesIn(meta *tableMeta, v RowValues) []*string {
	out := make([]*string, len(meta.columns))
	for i, c := range meta.columns {
		out[i] = v[c.Name]
	}
	return out
}
