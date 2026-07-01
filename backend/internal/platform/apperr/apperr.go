// Package apperr defines a small, transport-agnostic error model shared across
// layers. Domain and application code return *apperr.Error values; the HTTP
// layer maps their Kind to a status code. This keeps HTTP concerns out of the
// core and gives a single, consistent error contract.
package apperr

import "errors"

// Kind classifies an error for transport mapping.
type Kind int

const (
	KindInternal     Kind = iota // unexpected failure    -> 500
	KindInvalid                  // validation failure     -> 422
	KindNotFound                 // resource missing       -> 404
	KindConflict                 // uniqueness/state        -> 409
	KindUnauthorized             // missing/invalid auth   -> 401
	KindForbidden                // authenticated, denied  -> 403
)

// Error is the canonical application error.
type Error struct {
	Kind  Kind
	Msg   string
	Field string // optional: the offending field for validation errors
	Err   error  // optional: wrapped cause
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

func (e *Error) Unwrap() error { return e.Err }

// Invalid builds a validation error tied to a field.
func Invalid(field, msg string) *Error { return &Error{Kind: KindInvalid, Field: field, Msg: msg} }

// NotFound builds a not-found error.
func NotFound(msg string) *Error { return &Error{Kind: KindNotFound, Msg: msg} }

// Conflict builds a conflict error.
func Conflict(msg string) *Error { return &Error{Kind: KindConflict, Msg: msg} }

// Unauthorized builds a 401 error (missing or invalid credentials).
func Unauthorized(msg string) *Error { return &Error{Kind: KindUnauthorized, Msg: msg} }

// Forbidden builds a 403 error (authenticated but not permitted).
func Forbidden(msg string) *Error { return &Error{Kind: KindForbidden, Msg: msg} }

// Internal wraps an unexpected error.
func Internal(err error) *Error { return &Error{Kind: KindInternal, Msg: "internal error", Err: err} }

// KindOf extracts the Kind from any error, defaulting to KindInternal.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindInternal
}
