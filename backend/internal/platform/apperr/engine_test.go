package apperr

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestFromEngineConnectionErrorsAreGeneric(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}
	cases := []struct {
		name    string
		err     error
		want    string
		mustNot string
	}{
		{"refused", fmt.Errorf("ping: %w", refused), "could not connect", "refused"},
		{"pgx dial", errors.New("failed to connect to `user=root database=postgres`: 10.0.0.9:5432 (10.0.0.9): dial error: timeout"), "could not connect", "10.0.0.9"},
		{"dns", &net.DNSError{Err: "no such host", Name: "internal.corp"}, "could not connect", "internal.corp"},
		{"pg auth", errors.New("failed to connect to `user=root`: server error: FATAL: password authentication failed for user \"root\""), "rejected the login", "FATAL"},
		{"mysql auth", errors.New("Error 1045 (28000): Access denied for user 'root'@'10.0.0.1' (using password: YES)"), "rejected the login", "10.0.0.1"},
		{"policy", errors.New("this database host is not allowed: 127.0.0.1"), "not allowed", "127.0.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FromEngine(tc.err, "instance").Error()
			if !strings.Contains(got, tc.want) || strings.Contains(got, tc.mustNot) {
				t.Errorf("FromEngine = %q; want it to contain %q and not %q", got, tc.want, tc.mustNot)
			}
		})
	}
}

func TestFromEngineKeepsPermissionErrors(t *testing.T) {
	got := FromEngine(errors.New("Error 1044 (42000): Access denied for user 'fleetdock_rw_x'@'%' to database 'other'"), "sql").Error()
	if !strings.Contains(got, "to database 'other'") {
		t.Errorf("a permission error must not be reported as a login failure: %q", got)
	}
}

func TestFromEngineKeepsSQLErrors(t *testing.T) {
	got := FromEngine(errors.New("Error 1064 (42000): You have an error in your SQL syntax"), "sql").Error()
	if !strings.Contains(got, "SQL syntax") {
		t.Errorf("SQL errors should stay visible, got %q", got)
	}
	got = FromEngine(errors.New("dial postgres://root:hunter2@db/x failed: bad thing"), "x").Error()
	if strings.Contains(got, "hunter2") {
		t.Errorf("password leaked: %q", got)
	}
}
