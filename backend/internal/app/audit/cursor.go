package auditapp

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	auditdom "github.com/Fleetdock/fleetdock/backend/internal/domain/audit"
)

// EncodeCursor turns a listing position into an opaque token. Both the time
// and the id are kept: events written in the same microsecond still page
// without gaps or repeats.
func EncodeCursor(c auditdom.Cursor) string {
	raw := c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor reverses EncodeCursor.
func DecodeCursor(s string) (auditdom.Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return auditdom.Cursor{}, err
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return auditdom.Cursor{}, fmt.Errorf("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return auditdom.Cursor{}, err
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return auditdom.Cursor{}, err
	}
	return auditdom.Cursor{CreatedAt: t, ID: u}, nil
}
