package engine

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// scramIterations matches PostgreSQL's default scram_iterations.
const scramIterations = 4096

// pgPasswordLiteral returns the SQL string literal to put after PASSWORD in
// CREATE/ALTER ROLE. Whenever possible it is a SCRAM-SHA-256 verifier computed
// here, so the plaintext never reaches the server — where it could otherwise
// land in the server log (log_statement) or pg_stat_statements.
//
// PostgreSQL normalises passwords with SASLprep before hashing; for printable
// ASCII that is the identity, so only those passwords are pre-hashed. Anything
// else falls back to the (escaped) plaintext and lets the server hash it.
func pgPasswordLiteral(password string) string {
	if isPrintableASCII(password) {
		if v, err := scramVerifier(password); err == nil {
			return "'" + v + "'"
		}
	}
	return "'" + strings.ReplaceAll(password, "'", "''") + "'"
}

// scramVerifier builds a PostgreSQL SCRAM-SHA-256 secret:
// SCRAM-SHA-256$<iterations>:<salt>$<StoredKey>:<ServerKey> (RFC 5802/7677).
func scramVerifier(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return scramVerifierWithSalt(password, salt, scramIterations)
}

func scramVerifierWithSalt(password string, salt []byte, iterations int) (string, error) {
	salted, err := pbkdf2.Key(sha256.New, password, salt, iterations, sha256.Size)
	if err != nil {
		return "", err
	}
	clientKey := hmacSHA256(salted, "Client Key")
	storedKey := sha256.Sum256(clientKey)
	serverKey := hmacSHA256(salted, "Server Key")
	b64 := base64.StdEncoding.EncodeToString
	return fmt.Sprintf("SCRAM-SHA-256$%d:%s$%s:%s",
		iterations, b64(salt), b64(storedKey[:]), b64(serverKey)), nil
}

func hmacSHA256(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return s != ""
}
