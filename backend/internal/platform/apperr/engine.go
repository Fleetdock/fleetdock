package apperr

import (
	"context"
	"errors"
	"net"
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
	if msg, ok := connectionFailure(err); ok {
		return Invalid(field, msg)
	}
	return Invalid(field, sanitizeEngineMsg(err.Error()))
}

// EngineMessage is the client-safe text for an engine error, for results that
// report failure in a field instead of as an error (e.g. a connection test).
func EngineMessage(err error) string {
	if err == nil {
		return ""
	}
	if msg, ok := connectionFailure(err); ok {
		return msg
	}
	return sanitizeEngineMsg(err.Error())
}

// connectionFailure maps network-level failures to fixed messages. Echoing the
// raw dial error ("connection refused" vs "i/o timeout" vs "no such host")
// would turn every instance form into a port scanner for the control plane's
// network, and the distinction rarely helps the user anyway.
func connectionFailure(err error) (string, bool) {
	msg := strings.ToLower(err.Error())
	switch {
	// A failed MySQL login is error 1045, "Access denied for user 'x'@'h'
	// (using password: YES)". Permission errors (1044, 1142, ...) also start
	// "Access denied for user" and must stay visible, so match the login form.
	case strings.Contains(msg, "password authentication failed"),
		strings.Contains(msg, "access denied for user") && strings.Contains(msg, "(using password"),
		strings.Contains(msg, "no pg_hba.conf entry"):
		return "the database rejected the login; check the username, password and allowed hosts", true
	case strings.Contains(msg, "host is not allowed"):
		return "this database host is not allowed", true
	case errors.Is(err, context.DeadlineExceeded) && isDialError(err, msg):
		return "could not connect to the database host (timed out)", true
	case isDialError(err, msg):
		return "could not connect to the database host; check the host, port and firewall", true
	}
	return "", false
}

func isDialError(err error, msg string) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	for _, s := range []string{"dial tcp", "connection refused", "no such host", "i/o timeout",
		"no route to host", "network is unreachable", "failed to connect to"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
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
