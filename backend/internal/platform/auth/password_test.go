package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashPasswordLimits(t *testing.T) {
	ok := strings.Repeat("a", MaxPasswordBytes)
	h, err := HashPassword(ok)
	if err != nil {
		t.Fatalf("72 bytes: %v", err)
	}
	if !CheckPassword(h, ok) || CheckPassword(h, ok[:71]) {
		t.Fatal("hash does not verify the password exactly")
	}
	for _, pw := range []string{strings.Repeat("a", MaxPasswordBytes+1), strings.Repeat("я", 37)} { // 74 bytes
		if _, err := HashPassword(pw); !errors.Is(err, ErrPasswordTooLong) {
			t.Errorf("%d bytes: err = %v, want ErrPasswordTooLong", len(pw), err)
		}
	}
}
