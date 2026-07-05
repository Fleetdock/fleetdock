// Package audit is the domain model for the append-only, hash-chained audit
// log. Each entry's hash covers the previous entry's hash, making the log
// tamper-evident.
package audit

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ActorType identifies who performed an action.
type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorToken  ActorType = "token"
	ActorSystem ActorType = "system"
)

// Entry is one recorded action.
type Entry struct {
	ID           int64
	ActorType    ActorType
	ActorID      *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	Metadata     map[string]any
	CreatedAt    time.Time
}

// ListFilter narrows audit listings.
type ListFilter struct {
	ActorID      *uuid.UUID
	ResourceType string
	ResourceID   *uuid.UUID
	Limit        int
	Offset       int
}

// Page is one page of entries plus the unpaginated total.
type Page struct {
	Items []*Entry
	Total int
}

// Repository is the persistence port for the audit log.
type Repository interface {
	// Append records an entry, computing its hash chain link. It sets e.ID,
	// e.CreatedAt on success.
	Append(ctx context.Context, e *Entry) error
	List(ctx context.Context, f ListFilter) (Page, error)
}
