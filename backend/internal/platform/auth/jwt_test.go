package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTRoundTrip(t *testing.T) {
	j := NewJWT("a-secret-of-some-length", time.Hour)
	tok, err := j.Issue("user-1", 7)
	if err != nil {
		t.Fatal(err)
	}
	sub, epoch, err := j.Verify(tok)
	if err != nil || sub != "user-1" || epoch != 7 {
		t.Fatalf("Verify = %q, %d, %v", sub, epoch, err)
	}
}

func TestJWTRejects(t *testing.T) {
	j := NewJWT("a-secret-of-some-length", time.Hour)
	good, _ := j.Issue("user-1", 1)

	expired, _ := NewJWT("a-secret-of-some-length", -time.Minute).Issue("user-1", 1)
	otherKey, _ := NewJWT("another-secret-entirely", time.Hour).Issue("user-1", 1)
	parts := strings.Split(good, ".")
	tampered := parts[0] + "." + strings.TrimRight(parts[1], "=") + "x." + parts[2]
	// "none" and asymmetric algorithms must not be accepted.
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "user-1", "exp": time.Now().Add(time.Hour).Unix()}).
		SignedString(jwt.UnsafeAllowNoneSignatureType)

	for name, tok := range map[string]string{
		"expired": expired, "other key": otherKey, "tampered": tampered, "alg none": none, "garbage": "not.a.jwt", "empty": "",
	} {
		if _, _, err := j.Verify(tok); err == nil {
			t.Errorf("%s token accepted", name)
		}
	}
}

func TestGenerateToken(t *testing.T) {
	full, prefix, hash, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(full, TokenPrefix) || len(full) != len(TokenPrefix)+48 {
		t.Errorf("token %q", full)
	}
	if !strings.HasPrefix(full, prefix) || len(prefix) != len(TokenPrefix)+6 {
		t.Errorf("prefix %q of %q", prefix, full)
	}
	if hash != HashToken(full) || len(hash) != 64 || strings.Contains(hash, full) {
		t.Errorf("hash %q", hash)
	}
	other, _, _, _ := GenerateToken()
	if other == full {
		t.Error("two tokens are equal")
	}
}
