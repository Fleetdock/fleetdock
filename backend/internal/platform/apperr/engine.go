package apperr

import (
	"regexp"
	"strings"
)

// maxEngineMsg caps how much of a driver message is echoed back to a client.
const maxEngineMsg = 300

// dsnPattern matches credentials embedded in a connection string, which some
// drivers include verbatim in their error text.
var dsnPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)([^:/@\s]+):([^@\s]*)@`)

// FromEngine surfaces database engine errors as user-actionable validation
// errors. Failures like a bad grant, a missing role, or an unreachable host
// are the user's to fix, so the message is worth showing — unlike a generic
// internal error, which the HTTP layer deliberately hides.
//
// Errors that already carry a Kind pass through untouched; only internal ones
// are reclassified. The message is sanitized first: driver text can embed the
// DSN it was connecting with, password included.
func FromEngine(err error, field string) error {
	if err == nil {
		return nil
	}
	if KindOf(err) != KindInternal {
		return err
	}
	return Invalid(field, sanitizeEngineMsg(err.Error()))
}

func sanitizeEngineMsg(msg string) string {
	msg = strings.TrimSpace(dsnPattern.ReplaceAllString(msg, "$1$2:***@"))
	if len(msg) > maxEngineMsg {
		msg = strings.TrimSpace(msg[:maxEngineMsg]) + "…"
	}
	if msg == "" {
		return "the database rejected the request"
	}
	return msg
}
