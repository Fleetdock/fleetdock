// Package dbcredential is the domain model for application database credentials.
package dbcredential

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// AccessLevel is a permission profile for an application credential.
type AccessLevel string

const (
	AccessReadonly  AccessLevel = "readonly"
	AccessReadWrite AccessLevel = "readwrite"
	AccessAdmin     AccessLevel = "admin"
	AccessCustom    AccessLevel = "custom"
)

// Credential is a Fleetdock-managed application database user.
type Credential struct {
	ID          uuid.UUID
	DatabaseID  uuid.UUID
	Name        string
	Username    string
	SecretRef   string
	AccessLevel AccessLevel
	AccountHost string
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
}

// usernamePattern is the identifier shape both Postgres and MariaDB accept
// unquoted, so a generated or user-supplied name can never produce invalid SQL.
var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// GenerateUsername derives a unique account name from a credential name. The
// prefix is slugified so "My App!" yields something like "my_app_3f9a1c".
func GenerateUsername(name string) (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	suffix := hex.EncodeToString(buf)

	prefix := slugify(name)
	if prefix == "" {
		prefix = "app"
	}
	// Leave room for the separator and suffix within the 63-character limit.
	if max := 62 - len(suffix); len(prefix) > max {
		prefix = strings.TrimRight(prefix[:max], "_")
	}
	return prefix + "_" + suffix, nil
}

// slugify lowercases and replaces every character outside [a-z0-9_] with "_",
// collapsing runs and ensuring the result starts with a letter.
func slugify(s string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return ""
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "a" + out
	}
	return out
}

// NewCredential validates input for a new credential record. An empty username
// is generated from name — callers are not required to supply one.
func NewCredential(databaseID uuid.UUID, name, username string, access AccessLevel, accountHost string) (*Credential, error) {
	name = strings.TrimSpace(name)
	username = strings.TrimSpace(username)
	if name == "" {
		return nil, apperr.Invalid("name", "name is required")
	}
	if len(name) > 64 {
		return nil, apperr.Invalid("name", "name must be at most 64 characters")
	}
	if username == "" {
		generated, err := GenerateUsername(name)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		username = generated
	}
	if !usernamePattern.MatchString(username) {
		return nil, apperr.Invalid("username",
			"username must start with a letter and contain only lowercase letters, digits, and underscores")
	}
	if !access.Valid() {
		return nil, apperr.Invalid("access_level", "invalid access level")
	}
	accountHost = strings.TrimSpace(accountHost)
	if accountHost == "" {
		accountHost = "%"
	}
	return &Credential{
		ID:          uuid.New(),
		DatabaseID:  databaseID,
		Name:        name,
		Username:    username,
		AccessLevel: access,
		AccountHost: accountHost,
	}, nil
}

func (a AccessLevel) Valid() bool {
	switch a {
	case AccessReadonly, AccessReadWrite, AccessAdmin, AccessCustom:
		return true
	}
	return false
}

// IsActive reports whether the credential can authenticate.
func (c *Credential) IsActive(now time.Time) bool {
	if c.RevokedAt != nil {
		return false
	}
	if c.ExpiresAt != nil && !c.ExpiresAt.After(now) {
		return false
	}
	return true
}
