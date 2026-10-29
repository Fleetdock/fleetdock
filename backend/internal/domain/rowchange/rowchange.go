// Package rowchange is the data browser's change journal: what each edited
// row looked like before and after, kept for a while so edits can be undone.
package rowchange

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// Sources of a change.
const (
	SourceEditor = "editor"
	SourceUndo   = "undo"
)

// MaxEntryBytes caps one entry's encoded images. A bigger change is still
// listed (who, when, which row) but cannot be undone.
const MaxEntryBytes = 256 << 10

// MaxBatchUndo caps how many changes one batch undo reverts.
const MaxBatchUndo = 200

// Image is a row's values by column. It encodes as a JSON object whose
// values are null, a string, or {"b64": "..."} for values that are not
// valid UTF-8 or contain NUL (binary data), which jsonb cannot hold as text.
type Image engine.RowValues

// MarshalJSON implements json.Marshaler.
func (im Image) MarshalJSON() ([]byte, error) {
	if im == nil {
		return []byte("null"), nil
	}
	out := make(map[string]any, len(im))
	for c, v := range im {
		switch {
		case v == nil:
			out[c] = nil
		case !utf8.ValidString(*v) || strings.IndexByte(*v, 0) >= 0:
			out[c] = map[string]string{"b64": base64.StdEncoding.EncodeToString([]byte(*v))}
		default:
			out[c] = *v
		}
	}
	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (im *Image) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if raw == nil {
		*im = nil
		return nil
	}
	out := make(Image, len(raw))
	for c, v := range raw {
		if string(v) == "null" {
			out[c] = nil
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			out[c] = &s
			continue
		}
		var bin struct {
			B64 string `json:"b64"`
		}
		if err := json.Unmarshal(v, &bin); err != nil {
			return fmt.Errorf("column %q: %w", c, err)
		}
		dec, err := base64.StdEncoding.DecodeString(bin.B64)
		if err != nil {
			return fmt.Errorf("column %q: %w", c, err)
		}
		s = string(dec)
		out[c] = &s
	}
	*im = out
	return nil
}

// Values returns the image as engine values.
func (im Image) Values() engine.RowValues { return engine.RowValues(im) }

// Change is one journaled row change.
type Change struct {
	ID         uuid.UUID         `json:"id"`
	DatabaseID uuid.UUID         `json:"database_id"`
	Table      string            `json:"table"`
	Op         string            `json:"op"`
	Key        Image             `json:"key"`
	Before     Image             `json:"before"`
	After      Image             `json:"after"`
	Columns    []string          `json:"columns"`
	Session    map[string]string `json:"-"`
	Undoable   bool              `json:"undoable"`
	Note       string            `json:"note,omitempty"`
	BatchID    *uuid.UUID        `json:"batch_id,omitempty"`
	Source     string            `json:"source"`
	UserID     *uuid.UUID        `json:"user_id,omitempty"`
	// UserEmail is filled by List for display.
	UserEmail string     `json:"user_email,omitempty"`
	UndoOf    *uuid.UUID `json:"undo_of,omitempty"`
	UndoneAt  *time.Time `json:"undone_at,omitempty"`
	UndoneBy  *uuid.UUID `json:"undone_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// FromEngine builds a journal entry from an engine change. If the images
// are too big to keep, it keeps the row key only and marks it not undoable.
func FromEngine(databaseID uuid.UUID, table, source string, ch *engine.RowChange) *Change {
	c := &Change{
		DatabaseID: databaseID, Table: table, Op: ch.Op, Source: source,
		Key: Image(ch.Key), Before: Image(ch.Before), After: Image(ch.After),
		Columns: ch.Columns, Session: ch.Session, Undoable: ch.Undoable, Note: ch.Note,
	}
	if c.Columns == nil {
		c.Columns = []string{}
	}
	if c.Session == nil {
		c.Session = map[string]string{}
	}
	if size(c.Key)+size(c.Before)+size(c.After) > MaxEntryBytes {
		c.Before, c.After, c.Undoable = nil, nil, false
		c.Note = "the row is too large to keep a copy of, so this change cannot be undone"
	}
	return c
}

func size(im Image) int {
	b, err := json.Marshal(im)
	if err != nil {
		return MaxEntryBytes + 1
	}
	return len(b)
}

// ListFilter selects journal entries, newest first.
type ListFilter struct {
	DatabaseID uuid.UUID
	Table      string
	BatchID    *uuid.UUID
	// Before is a cursor: entries created strictly before it.
	Before *time.Time
	Limit  int
}

// Repository persists the journal.
type Repository interface {
	Add(ctx context.Context, c *Change) error
	Get(ctx context.Context, id uuid.UUID) (*Change, error)
	List(ctx context.Context, f ListFilter) ([]Change, error)
	// ClaimUndo marks an entry undone by user unless it already is; false
	// means someone else got there first.
	ClaimUndo(ctx context.Context, id, user uuid.UUID, at time.Time) (bool, error)
	// ReleaseUndo clears a claim whose undo failed.
	ReleaseUndo(ctx context.Context, id uuid.UUID) error
	// PruneBefore deletes entries older than cutoff and returns how many.
	PruneBefore(ctx context.Context, cutoff time.Time) (int64, error)
}
