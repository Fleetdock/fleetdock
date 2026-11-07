package userapp

import (
	"strings"
	"testing"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

func TestValidatePassword(t *testing.T) {
	for pw, ok := range map[string]bool{
		"short":                  false,
		"long-enough":            true,
		strings.Repeat("a", 72):  true,
		strings.Repeat("a", 73):  false, // a 500 before: bcrypt refuses it
		strings.Repeat("ü", 36):  true,  // 72 bytes
		strings.Repeat("ü", 37):  false, // 74 bytes, 37 letters
		strings.Repeat("a", 128): false,
	} {
		err := validatePassword(pw)
		if (err == nil) != ok {
			t.Errorf("%d bytes: err = %v, want ok=%v", len(pw), err, ok)
		}
		if err != nil && apperr.KindOf(err) != apperr.KindInvalid {
			t.Errorf("%d bytes: %v is not a validation error", len(pw), err)
		}
	}
}
