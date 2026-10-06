package engine

import (
	"strings"
)

// SplitStatements splits a console script into statements on semicolons that
// are outside string literals, quoted identifiers, comments and (PostgreSQL)
// dollar-quoted bodies. A MySQL-client style "DELIMITER xx" line changes the
// terminator, so CREATE PROCEDURE bodies containing semicolons can be run.
// Empty statements (and comment-only ones) are dropped.
func SplitStatements(script string) []string {
	var out []string
	var cur strings.Builder
	delim := ";"
	s := script
	i := 0

	flush := func() {
		stmt := strings.TrimSpace(cur.String())
		cur.Reset()
		if stmt != "" && leadingKeyword(stmt) != "" {
			out = append(out, stmt)
		}
	}

	for i < len(s) {
		// DELIMITER directive: at the start of a line, between statements.
		if (i == 0 || s[i-1] == '\n') && strings.TrimSpace(cur.String()) == "" {
			if d, n, ok := delimiterDirective(s[i:]); ok {
				cur.Reset()
				delim = d
				i += n
				continue
			}
		}
		c := s[i]

		switch {
		case strings.HasPrefix(s[i:], delim):
			flush()
			i += len(delim)
			continue
		case c == '\'' || c == '"' || c == '`':
			j := skipQuoted(s, i, c)
			cur.WriteString(s[i:j])
			i = j
			continue
		case c == '-' && strings.HasPrefix(s[i:], "--"), c == '#':
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				j = len(s) - i
			}
			cur.WriteString(s[i : i+j])
			i += j
			continue
		case c == '/' && strings.HasPrefix(s[i:], "/*"):
			j := strings.Index(s[i+2:], "*/")
			end := len(s)
			if j >= 0 {
				end = i + 2 + j + 2
			}
			cur.WriteString(s[i:end])
			i = end
			continue
		case c == '$':
			if tag, ok := dollarTag(s[i:]); ok {
				end := strings.Index(s[i+len(tag):], tag)
				stop := len(s)
				if end >= 0 {
					stop = i + len(tag) + end + len(tag)
				}
				cur.WriteString(s[i:stop])
				i = stop
				continue
			}
		}
		cur.WriteByte(c)
		i++
	}
	flush()
	return out
}

// delimiterDirective parses "DELIMITER <token>" at the start of s, returning
// the token and the length consumed (through the end of the line).
func delimiterDirective(s string) (string, int, bool) {
	t := strings.TrimLeft(s, " \t")
	skipped := len(s) - len(t)
	if len(t) < 10 || !strings.EqualFold(t[:9], "DELIMITER") || (t[9] != ' ' && t[9] != '\t') {
		return "", 0, false
	}
	line := t
	n := strings.IndexByte(t, '\n')
	if n >= 0 {
		line = t[:n]
	}
	tok := strings.TrimSpace(line[9:])
	if tok == "" || strings.ContainsAny(tok, " \t") {
		return "", 0, false
	}
	consumed := skipped + len(line)
	if n >= 0 {
		consumed++
	}
	return tok, consumed, true
}

// skipQuoted returns the index just past the literal starting at s[i]. A
// doubled quote is an escaped quote; in single-quoted strings a backslash
// escapes the next byte (MySQL; harmless for PostgreSQL's E” strings).
func skipQuoted(s string, i int, q byte) int {
	j := i + 1
	for j < len(s) {
		switch {
		case s[j] == '\\' && q == '\'':
			j += 2
			continue
		case s[j] == q:
			if j+1 < len(s) && s[j+1] == q {
				j += 2
				continue
			}
			return j + 1
		}
		j++
	}
	return len(s)
}

// dollarTag recognises a PostgreSQL dollar-quote opener ($$ or $tag$) at the
// start of s. $1-style parameters are not tags.
func dollarTag(s string) (string, bool) {
	if len(s) < 2 {
		return "", false
	}
	if s[1] == '$' {
		return "$$", true
	}
	j := 1
	for j < len(s) && (s[j] == '_' || (s[j] >= 'a' && s[j] <= 'z') || (s[j] >= 'A' && s[j] <= 'Z') || (j > 1 && s[j] >= '0' && s[j] <= '9')) {
		j++
	}
	if j > 1 && j < len(s) && s[j] == '$' {
		return s[:j+1], true
	}
	return "", false
}
