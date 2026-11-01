package auditapp

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	auditdom "github.com/Fleetdock/fleetdock/backend/internal/domain/audit"
)

type fakeRepo struct {
	events []auditdom.Event
	last   auditdom.Filter
}

func (f *fakeRepo) Append(_ context.Context, e *auditdom.Event) error {
	f.events = append(f.events, *e)
	return nil
}

func (f *fakeRepo) List(_ context.Context, fl auditdom.Filter) ([]auditdom.Event, error) {
	f.last = fl
	out := f.events
	if len(out) > fl.Limit {
		out = out[:fl.Limit]
	}
	return out, nil
}

func (f *fakeRepo) PruneBefore(context.Context, time.Time) (int64, error) { return 0, nil }

func (f *fakeRepo) ResourceName(_ context.Context, typ string, _ uuid.UUID) string {
	return typ + "-name"
}

func TestCursorRoundTrip(t *testing.T) {
	c := auditdom.Cursor{CreatedAt: time.Date(2026, 11, 1, 10, 0, 0, 123456000, time.UTC), ID: uuid.New()}
	got, err := DecodeCursor(EncodeCursor(c))
	if err != nil || !got.CreatedAt.Equal(c.CreatedAt) || got.ID != c.ID {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
	if _, err := DecodeCursor("not-a-cursor"); err == nil {
		t.Error("garbage cursor accepted")
	}
}

func TestRecordUsesRequestAndMarksEntry(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo)
	uid, tok := uuid.New(), uuid.New()
	ctx := WithRequest(context.Background(), Request{UserID: &uid, Email: "a@x.dev", Kind: auditdom.ActorAPIToken, TokenID: &tok, IP: "10.0.0.1", UserAgent: "curl"})
	ctx, e := WithEntry(ctx)
	db := uuid.New()
	if err := s.Record(ctx, nil, "credential.create", "database", &db, map[string]any{"name": "app"}); err != nil {
		t.Fatal(err)
	}
	if !e.State().Recorded {
		t.Error("entry not marked recorded")
	}
	ev := repo.events[0]
	if *ev.ActorUserID != uid || ev.ActorEmail != "a@x.dev" || ev.ActorKind != auditdom.ActorAPIToken ||
		*ev.TokenID != tok || ev.IP != "10.0.0.1" || ev.ResourceName != "database-name" || ev.Outcome != "success" {
		t.Errorf("event = %+v", ev)
	}
}

func TestAppendWithoutRequestIsSystem(t *testing.T) {
	repo := &fakeRepo{}
	if err := NewService(repo).Append(context.Background(), &auditdom.Event{Action: "user.password_reset", ResourceType: "user"}, nil); err != nil {
		t.Fatal(err)
	}
	if ev := repo.events[0]; ev.ActorKind != auditdom.ActorSystem || string(ev.Metadata) != "{}" {
		t.Errorf("event = %+v", ev)
	}
}

func TestListPages(t *testing.T) {
	repo := &fakeRepo{}
	t0 := time.Now()
	for i := 0; i < 3; i++ {
		repo.events = append(repo.events, auditdom.Event{ID: uuid.New(), CreatedAt: t0.Add(-time.Duration(i) * time.Second)})
	}
	s := NewService(repo)
	p, err := s.List(context.Background(), auditdom.Filter{Limit: 2}, "")
	if err != nil || len(p.Items) != 2 || p.NextCursor == "" || repo.last.Limit != 3 {
		t.Fatalf("page = %+v, %v (limit asked %d)", p, err, repo.last.Limit)
	}
	c, _ := DecodeCursor(p.NextCursor)
	if c.ID != repo.events[1].ID {
		t.Errorf("cursor = %+v", c)
	}
	p, _ = s.List(context.Background(), auditdom.Filter{Limit: 5}, "")
	if len(p.Items) != 3 || p.NextCursor != "" {
		t.Errorf("last page = %+v", p)
	}
	if _, err := s.List(context.Background(), auditdom.Filter{}, "%%%"); err == nil {
		t.Error("bad cursor accepted")
	}
}
