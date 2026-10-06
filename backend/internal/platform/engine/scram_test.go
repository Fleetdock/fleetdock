package engine

import (
	"strings"
	"testing"
)

// The reference verifier for password "pencil", salt "QSXCR+Q6sek8bf92"
// (base64) and 4096 iterations was computed independently with Python's
// hashlib.pbkdf2_hmac + hmac, following RFC 5802; it pins the derivation.
func TestScramVerifierKnownVector(t *testing.T) {
	salt := []byte{0x41, 0x25, 0xc2, 0x47, 0xe4, 0x3a, 0xb1, 0xe9, 0x3c, 0x6d, 0xff, 0x76}
	got, err := scramVerifierWithSalt("pencil", salt, 4096)
	if err != nil {
		t.Fatal(err)
	}
	want := "SCRAM-SHA-256$4096:QSXCR+Q6sek8bf92$" +
		"FO+9jBb3MUukt6jJnzjPZOWc5ow/Pu6JtPyju0aqaE8=:qxJ1SbmSAi5EcS0J5Ck/cKAm/+Ixa+Kwp63f4OHDgzo="
	if got != want {
		t.Fatalf("verifier = %q, want %q", got, want)
	}
}

func TestPGPasswordLiteral(t *testing.T) {
	if lit := pgPasswordLiteral("s3cret'pw"); !strings.HasPrefix(lit, "'SCRAM-SHA-256$4096:") || strings.Contains(lit, "s3cret") {
		t.Errorf("ASCII password should be sent as a verifier, got %s", lit)
	}
	if lit := pgPasswordLiteral("пароль'"); lit != "'пароль'''" {
		t.Errorf("non-ASCII password should fall back to escaped plaintext, got %s", lit)
	}
}
