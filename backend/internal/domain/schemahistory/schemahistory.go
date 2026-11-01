// Package schemahistory models the recorded structure of watched databases:
// a snapshot for every distinct structure, so changes can be listed, diffed
// and alerted on.
package schemahistory

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/schemadiff"
)

// Triggers record why a snapshot was taken.
const (
	TriggerScheduled = "scheduled"
	TriggerManual    = "manual"
	TriggerDDL       = "ddl"
)

// Limits.
const (
	// MaxStoredBytes caps the uncompressed JSON kept per snapshot; larger
	// structures are recorded by hash only.
	MaxStoredBytes = 16 << 20
	// Keep is how many snapshots are kept per database.
	Keep = 200
)

// Watch is a database's schema-history setting.
type Watch struct {
	DatabaseID    uuid.UUID  `json:"database_id"`
	Enabled       bool       `json:"enabled"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	LastError     *string    `json:"last_error,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Summary describes a snapshot without its full structure.
type Summary struct {
	Counts schemadiff.Counts `json:"counts"`
	// Changes tallies the differences from the previous snapshot.
	Changes schemadiff.Tally `json:"changes"`
	// Highlights name the first few changes ("+ table orders").
	Highlights []string `json:"highlights,omitempty"`
	Truncated  bool     `json:"truncated,omitempty"`
	// Stored is false when the structure was too large to keep.
	Stored bool `json:"stored"`
}

// Snapshot is one recorded structure.
type Snapshot struct {
	ID         uuid.UUID  `json:"id"`
	DatabaseID uuid.UUID  `json:"database_id"`
	Hash       string     `json:"hash"`
	Summary    Summary    `json:"summary"`
	PreviousID *uuid.UUID `json:"previous_id,omitempty"`
	Trigger    string     `json:"trigger"`
	TakenBy    *uuid.UUID `json:"taken_by,omitempty"`
	TakenAt    time.Time  `json:"taken_at"`
	// SchemaGz is the gzip-compressed JSON structure (nil if not kept or not
	// loaded).
	SchemaGz []byte `json:"-"`
}

// Encode compresses a structure for storage; nil when it is too large.
func Encode(s *schemadiff.Schema) ([]byte, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxStoredBytes {
		return nil, nil
	}
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Decode reverses Encode.
func Decode(gz []byte) (*schemadiff.Schema, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	raw, err := io.ReadAll(io.LimitReader(zr, MaxStoredBytes+1))
	if err != nil {
		return nil, err
	}
	var s schemadiff.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Repository persists watches and snapshots.
type Repository interface {
	// GetWatch returns the database's watch, or nil.
	GetWatch(ctx context.Context, databaseID uuid.UUID) (*Watch, error)
	SetWatch(ctx context.Context, databaseID uuid.UUID, enabled bool, by *uuid.UUID) (*Watch, error)
	// ListDue lists enabled watches last checked before the cutoff.
	ListDue(ctx context.Context, checkedBefore time.Time, limit int) ([]uuid.UUID, error)
	MarkChecked(ctx context.Context, databaseID uuid.UUID, at time.Time, errMsg string) error
	// Latest returns the newest snapshot with its structure, or nil.
	Latest(ctx context.Context, databaseID uuid.UUID) (*Snapshot, error)
	// Add stores a snapshot and drops the oldest beyond Keep.
	Add(ctx context.Context, s *Snapshot) error
	// List returns snapshots newest first, without their structure.
	List(ctx context.Context, databaseID uuid.UUID, limit int) ([]Snapshot, error)
	// Get returns one snapshot of the database with its structure.
	Get(ctx context.Context, databaseID, id uuid.UUID) (*Snapshot, error)
}
