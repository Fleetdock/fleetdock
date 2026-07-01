package postgres

import "strings"

// join renders SQL WHERE conditions joined by AND.
func join(conds []string) string { return strings.Join(conds, " AND ") }
