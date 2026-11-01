// Package audit is the domain model for append-only audit events.
package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Actor kinds: who performed an action.
const (
	ActorUser      = "user"      // a dashboard session
	ActorAPIToken  = "api_token" // an API token (TokenID says which)
	ActorSystem    = "system"    // the CLI or Fleetdock itself
	ActorAnonymous = "anonymous" // no session, e.g. a failed login
)

// Outcomes.
const (
	OutcomeSuccess = "success"
	OutcomeDenied  = "denied"  // refused for lack of permission
	OutcomeFailure = "failure" // e.g. a wrong password
)

// Event is one immutable audit record.
type Event struct {
	ID           uuid.UUID       `json:"id"`
	ActorUserID  *uuid.UUID      `json:"actor_user_id,omitempty"`
	ActorEmail   string          `json:"actor_email,omitempty"`
	ActorKind    string          `json:"actor_kind"`
	TokenID      *uuid.UUID      `json:"token_id,omitempty"`
	IP           string          `json:"ip,omitempty"`
	UserAgent    string          `json:"user_agent,omitempty"`
	Action       string          `json:"action"`
	Outcome      string          `json:"outcome"`
	ResourceType string          `json:"resource_type"`
	ResourceID   *uuid.UUID      `json:"resource_id,omitempty"`
	ResourceName string          `json:"resource_name,omitempty"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    time.Time       `json:"created_at"`
}

// Cursor is a position in the newest-first listing.
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// Filter narrows a listing. Zero values do not filter.
type Filter struct {
	ActorUserID *uuid.UUID
	// ActionPrefix matches an action exactly, or a category when it ends
	// with "." ("user." matches user.create, user.delete…).
	ActionPrefix string
	ResourceType string
	ResourceID   *uuid.UUID
	Outcome      string
	Since, Until time.Time
	// After continues a listing: only events older than the cursor.
	After *Cursor
	Limit int
}

// Repository persists audit events.
type Repository interface {
	Append(ctx context.Context, e *Event) error
	// List returns events newest first.
	List(ctx context.Context, f Filter) ([]Event, error)
	// PruneBefore deletes events older than t and returns how many.
	PruneBefore(ctx context.Context, t time.Time) (int64, error)
	// ResourceName looks up the current display name of a resource, or ""
	// when it has none or no longer exists.
	ResourceName(ctx context.Context, resourceType string, id uuid.UUID) string
}
