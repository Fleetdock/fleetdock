// Package httpapi is the HTTP transport layer. It translates requests into
// application use-case calls and maps results and errors back to HTTP.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// writeError maps an application error to a consistent HTTP error response.
func writeError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal"
	switch apperr.KindOf(err) {
	case apperr.KindInvalid:
		status, code = http.StatusUnprocessableEntity, "invalid"
	case apperr.KindNotFound:
		status, code = http.StatusNotFound, "not_found"
	case apperr.KindConflict:
		status, code = http.StatusConflict, "conflict"
	case apperr.KindUnauthorized:
		status, code = http.StatusUnauthorized, "unauthorized"
	case apperr.KindForbidden:
		status, code = http.StatusForbidden, "forbidden"
	}

	detail := errorDetail{Code: code, Message: "internal server error"}
	var ae *apperr.Error
	if errors.As(err, &ae) {
		detail.Field = ae.Field
		if ae.Kind != apperr.KindInternal {
			detail.Message = ae.Msg
		}
	}
	if status >= http.StatusInternalServerError {
		slog.Error("request failed", "error", err.Error())
	}
	writeJSON(w, status, errorBody{Error: detail})
}
