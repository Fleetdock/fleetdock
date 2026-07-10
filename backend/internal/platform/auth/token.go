package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// TokenPrefix identifies API tokens issued by this platform.
const TokenPrefix = "fleetd_"

// GenerateToken creates a new random API token. It returns the full secret
// (shown to the user exactly once), a non-secret display prefix, and the
// sha256 hash that is stored.
func GenerateToken() (full, prefix, hash string, err error) {
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	full = TokenPrefix + hex.EncodeToString(buf)
	prefix = full[:len(TokenPrefix)+6]
	hash = HashToken(full)
	return full, prefix, hash, nil
}

// HashToken returns the hex sha256 of a full token, used for storage/lookup.
func HashToken(full string) string {
	sum := sha256.Sum256([]byte(full))
	return hex.EncodeToString(sum[:])
}
