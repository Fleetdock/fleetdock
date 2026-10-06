package postgres

import "strings"

// join renders SQL WHERE conditions joined by AND.
func join(conds []string) string { return strings.Join(conds, " AND ") }

// joinSet renders SQL UPDATE assignments joined by commas.
func joinSet(sets []string) string { return strings.Join(sets, ", ") }

// likeEscape escapes LIKE wildcards so user search text matches literally
// (PostgreSQL's default LIKE escape character is the backslash).
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
