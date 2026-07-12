package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWT issues and verifies HS256 tokens carrying a user id in the subject and a
// token epoch used for session invalidation.
type JWT struct {
	secret []byte
	ttl    time.Duration
}

// claims are the signed token claims: the standard registered set plus the
// user's token epoch at issue time.
type claims struct {
	jwt.RegisteredClaims
	Epoch int `json:"epoch"`
}

// NewJWT builds a JWT signer/verifier.
func NewJWT(secret string, ttl time.Duration) *JWT {
	return &JWT{secret: []byte(secret), ttl: ttl}
}

// Issue returns a signed token for the given subject (user id) and token epoch.
func (j *JWT) Issue(subject string, epoch int) (string, error) {
	now := time.Now()
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.ttl)),
		},
		Epoch: epoch,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(j.secret)
}

// Verify validates a token and returns its subject (user id) and token epoch.
func (j *JWT) Verify(token string) (subject string, epoch int, err error) {
	parsed, err := jwt.ParseWithClaims(token, &claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return j.secret, nil
	})
	if err != nil {
		return "", 0, err
	}
	c, ok := parsed.Claims.(*claims)
	if !ok || !parsed.Valid {
		return "", 0, fmt.Errorf("invalid token")
	}
	return c.Subject, c.Epoch, nil
}
