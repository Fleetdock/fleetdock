package database

import (
	"testing"

	"github.com/google/uuid"
)

func TestScratchNames(t *testing.T) {
	id := uuid.MustParse("0123abcd-4567-89ef-0123-456789abcdef")
	if got := ScratchName("fds", id); got != "fds_0123abcd456789ef" {
		t.Fatalf("ScratchName = %q", got)
	}
	for _, n := range []string{"fdv_0123abcd456789ef", "fds_0123abcd456789ef"} {
		if !IsScratchName(n) {
			t.Errorf("%s should be reserved", n)
		}
		if _, err := NewDatabase(uuid.New(), n, "", "", nil, nil); err == nil {
			t.Errorf("%s must not be registrable", n)
		}
	}
	// Similar but ordinary names stay usable.
	for _, n := range []string{"fds_reports", "fdv_0123ABCD456789EF", "fds_0123abcd456789e", "xfds_0123abcd456789ef"} {
		if IsScratchName(n) {
			t.Errorf("%s should not be reserved", n)
		}
	}
}
