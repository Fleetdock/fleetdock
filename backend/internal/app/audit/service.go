// Package auditapp records and reads append-only audit events.
//
// Most events are written by the HTTP layer, once per audited request (see
// httpapi/audit.go): it puts the caller (Request) and an Entry into the
// request context, runs the handler, and records the outcome. Handlers and
// services add detail to that Entry through the helpers below; a service that
// writes its own, more specific event with Record marks the Entry recorded so
// the request is not logged twice.
package auditapp

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"

	auditdom "github.com/Fleetdock/fleetdock/backend/internal/domain/audit"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// Service appends and lists audit events.
type Service struct {
	repo auditdom.Repository
}

// NewService wires the audit service.
func NewService(repo auditdom.Repository) *Service {
	return &Service{repo: repo}
}

// Request is who made a request and from where.
type Request struct {
	UserID    *uuid.UUID
	Email     string
	Kind      string // auditdom.Actor*
	TokenID   *uuid.UUID
	IP        string
	UserAgent string
}

type ctxKey int

const (
	requestKey ctxKey = iota
	entryKey
)

// WithRequest attaches the caller of the current request.
func WithRequest(ctx context.Context, r Request) context.Context {
	return context.WithValue(ctx, requestKey, r)
}

// RequestFrom returns the caller attached by WithRequest.
func RequestFrom(ctx context.Context) (Request, bool) {
	r, ok := ctx.Value(requestKey).(Request)
	return r, ok
}

// Entry collects what a handler knows about the event its request produces.
// It is safe for concurrent use.
type Entry struct {
	mu           sync.Mutex
	resourceID   *uuid.UUID
	resourceName string
	meta         map[string]any
	actor        *Request
	skip         bool
	recorded     bool
	failed       bool
}

// WithEntry starts collecting an event for the current request.
func WithEntry(ctx context.Context) (context.Context, *Entry) {
	e := &Entry{}
	return context.WithValue(ctx, entryKey, e), e
}

func entry(ctx context.Context) *Entry {
	e, _ := ctx.Value(entryKey).(*Entry)
	return e
}

// SetResource names the resource the request acted on (a created object's
// id, or a better name than the one looked up).
func SetResource(ctx context.Context, id *uuid.UUID, name string) {
	if e := entry(ctx); e != nil {
		e.mu.Lock()
		if id != nil {
			e.resourceID = id
		}
		if name != "" {
			e.resourceName = name
		}
		e.mu.Unlock()
	}
}

// AddMeta adds secret-free detail to the request's event.
func AddMeta(ctx context.Context, key string, value any) {
	if e := entry(ctx); e != nil {
		e.mu.Lock()
		if e.meta == nil {
			e.meta = map[string]any{}
		}
		e.meta[key] = value
		e.mu.Unlock()
	}
}

// SetActor overrides who the event is attributed to: the login handler
// knows the user only once the password checked out (or only the email
// that was tried).
func SetActor(ctx context.Context, userID *uuid.UUID, email string) {
	if e := entry(ctx); e != nil {
		e.mu.Lock()
		e.actor = &Request{UserID: userID, Email: email}
		e.mu.Unlock()
	}
}

// Fail records the request as a failure (a wrong password) rather than
// dropping it with the other client errors.
func Fail(ctx context.Context) {
	if e := entry(ctx); e != nil {
		e.mu.Lock()
		e.failed = true
		e.mu.Unlock()
	}
}

// Skip drops the request's event: the request turned out not to be worth
// recording (a console script that only reads).
func Skip(ctx context.Context) {
	if e := entry(ctx); e != nil {
		e.mu.Lock()
		e.skip = true
		e.mu.Unlock()
	}
}

// EntryState is a snapshot of an Entry for the HTTP layer.
type EntryState struct {
	ResourceID   *uuid.UUID
	ResourceName string
	Meta         map[string]any
	Actor        *Request
	Skip         bool
	Recorded     bool
	Failed       bool
}

// State returns what was collected.
func (e *Entry) State() EntryState {
	e.mu.Lock()
	defer e.mu.Unlock()
	meta := make(map[string]any, len(e.meta))
	for k, v := range e.meta {
		meta[k] = v
	}
	return EntryState{
		ResourceID: e.resourceID, ResourceName: e.resourceName, Meta: meta,
		Actor: e.actor, Skip: e.skip, Recorded: e.recorded, Failed: e.failed,
	}
}

// Record stores one event with secret-free metadata. The caller is taken
// from the request context when there is one (actor may be nil then); an
// in-flight request's own event is suppressed in favour of this one.
func (s *Service) Record(ctx context.Context, actor *uuid.UUID, action, resourceType string, resourceID *uuid.UUID, metadata map[string]any) error {
	if e := entry(ctx); e != nil {
		e.mu.Lock()
		e.recorded = true
		e.mu.Unlock()
	}
	ev := &auditdom.Event{
		ActorUserID:  actor,
		Action:       action,
		Outcome:      auditdom.OutcomeSuccess,
		ResourceType: resourceType,
		ResourceID:   resourceID,
	}
	if resourceID != nil {
		ev.ResourceName = s.repo.ResourceName(ctx, resourceType, *resourceID)
	}
	return s.Append(ctx, ev, metadata)
}

// Append stores a fully described event, filling in the caller from the
// request context and the kind when they are missing.
func (s *Service) Append(ctx context.Context, ev *auditdom.Event, metadata map[string]any) error {
	if r, ok := RequestFrom(ctx); ok {
		if ev.ActorUserID == nil {
			ev.ActorUserID = r.UserID
		}
		if ev.ActorEmail == "" {
			ev.ActorEmail = r.Email
		}
		if ev.ActorKind == "" {
			ev.ActorKind = r.Kind
		}
		if ev.TokenID == nil {
			ev.TokenID = r.TokenID
		}
		if ev.IP == "" {
			ev.IP = r.IP
		}
		if ev.UserAgent == "" {
			ev.UserAgent = r.UserAgent
		}
	}
	if ev.ActorKind == "" {
		ev.ActorKind = auditdom.ActorUser
		if ev.ActorUserID == nil {
			ev.ActorKind = auditdom.ActorSystem
		}
	}
	if ev.Outcome == "" {
		ev.Outcome = auditdom.OutcomeSuccess
	}
	if ev.ID == uuid.Nil {
		ev.ID = uuid.New()
	}
	if len(ev.UserAgent) > 512 {
		ev.UserAgent = ev.UserAgent[:512]
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	meta, err := json.Marshal(metadata)
	if err != nil {
		meta = []byte("{}")
	}
	ev.Metadata = meta
	return s.repo.Append(ctx, ev)
}

// ResourceName looks up a resource's current display name.
func (s *Service) ResourceName(ctx context.Context, resourceType string, id uuid.UUID) string {
	return s.repo.ResourceName(ctx, resourceType, id)
}

// Page is one page of the newest-first listing.
type Page struct {
	Items []auditdom.Event `json:"items"`
	// NextCursor continues the listing; empty on the last page.
	NextCursor string `json:"next_cursor,omitempty"`
}

// Listing limits.
const (
	DefaultPageSize = 50
	MaxPageSize     = 100
	MaxExportRows   = 10000
)

// List returns one page of events. cursor is a NextCursor from the
// previous page, or empty.
func (s *Service) List(ctx context.Context, f auditdom.Filter, cursor string) (*Page, error) {
	if cursor != "" {
		c, err := DecodeCursor(cursor)
		if err != nil {
			return nil, apperr.Invalid("cursor", "invalid cursor")
		}
		f.After = &c
	}
	if f.Limit <= 0 {
		f.Limit = DefaultPageSize
	}
	if f.Limit > MaxPageSize {
		f.Limit = MaxPageSize
	}
	want := f.Limit
	f.Limit++ // one more tells whether there is a next page
	items, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, err
	}
	p := &Page{Items: items}
	if len(items) > want {
		p.Items = items[:want]
		last := p.Items[want-1]
		p.NextCursor = EncodeCursor(auditdom.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	if p.Items == nil {
		p.Items = []auditdom.Event{}
	}
	return p, nil
}

// Export returns up to MaxExportRows events matching f, newest first.
func (s *Service) Export(ctx context.Context, f auditdom.Filter) ([]auditdom.Event, error) {
	f.After = nil
	f.Limit = MaxExportRows
	return s.repo.List(ctx, f)
}

// Prune deletes events older than retention.
func (s *Service) Prune(ctx context.Context, retention time.Duration, now time.Time) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	return s.repo.PruneBefore(ctx, now.Add(-retention))
}
