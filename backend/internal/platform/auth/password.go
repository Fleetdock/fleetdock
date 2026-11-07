// Package auth provides password hashing and JWT issuing/verification.
package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// MaxPasswordBytes is the longest password bcrypt accepts: it reads only the
// first 72 bytes, and refuses longer input rather than truncate it silently.
// A UTF-8 letter outside ASCII takes two to four of them.
const MaxPasswordBytes = 72

// ErrPasswordTooLong is returned for a password over MaxPasswordBytes.
var ErrPasswordTooLong = fmt.Errorf("password must be at most %d bytes (%d plain letters, fewer with accents or non-Latin letters)",
	MaxPasswordBytes, MaxPasswordBytes)

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(plaintext string) (string, error) {
	if len(plaintext) > MaxPasswordBytes {
		return "", ErrPasswordTooLong
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return "", ErrPasswordTooLong
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword reports whether plaintext matches the stored bcrypt hash.
func CheckPassword(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
