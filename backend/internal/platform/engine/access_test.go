package engine

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsPermissionDenied(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{&mysql.MySQLError{Number: 1142, Message: "SELECT command denied"}, true},
		{fmt.Errorf("wrapped: %w", &mysql.MySQLError{Number: 1044}), true},
		{&mysql.MySQLError{Number: 1064, Message: "syntax"}, false},
		{&pgconn.PgError{Code: "42501"}, true},
		{&pgconn.PgError{Code: "42P01"}, false},
		{errors.New("permission denied"), false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := IsPermissionDenied(tc.err); got != tc.want {
			t.Errorf("IsPermissionDenied(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
