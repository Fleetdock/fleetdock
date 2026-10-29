package catalogapp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	catalogdom "github.com/Fleetdock/fleetdock/backend/internal/domain/catalog"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

type fakeRepo struct {
	mu       sync.Mutex
	due      []uuid.UUID
	replaced map[uuid.UUID]*engine.ColumnCatalog
	failed   map[uuid.UUID]string
	stale    time.Time
}

func (r *fakeRepo) Replace(_ context.Context, id uuid.UUID, cat *engine.ColumnCatalog, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replaced[id] = cat
	return nil
}

func (r *fakeRepo) Failed(_ context.Context, id uuid.UUID, msg string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed[id] = msg
	return nil
}

func (r *fakeRepo) ListDue(_ context.Context, staleBefore, _ time.Time, _ int) ([]uuid.UUID, error) {
	r.stale = staleBefore
	return r.due, nil
}

func (r *fakeRepo) Search(context.Context, string, int) ([]catalogdom.Hit, error) { return nil, nil }

type fakeSource struct {
	fail     map[uuid.UUID]bool
	inFlight atomic.Int32
	peak     atomic.Int32
}

func (s *fakeSource) AdminColumns(_ context.Context, id uuid.UUID) (*engine.ColumnCatalog, error) {
	n := s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	for {
		p := s.peak.Load()
		if n <= p || s.peak.CompareAndSwap(p, n) {
			break
		}
	}
	time.Sleep(5 * time.Millisecond)
	if s.fail[id] {
		return nil, errors.New("connection refused")
	}
	return &engine.ColumnCatalog{Tables: []engine.CatalogTable{{Name: "t"}}}, nil
}

func newRepo(due ...uuid.UUID) *fakeRepo {
	return &fakeRepo{due: due, replaced: map[uuid.UUID]*engine.ColumnCatalog{}, failed: map[uuid.UUID]string{}}
}

func TestRefreshDueDisabled(t *testing.T) {
	repo := newRepo(uuid.New())
	svc := NewService(repo, &fakeSource{}, 0)
	if svc.Enabled() || svc.RefreshDue(context.Background()) != 0 || len(repo.replaced) != 0 {
		t.Fatal("a zero interval must disable refreshes")
	}
}

func TestRefreshDueStoresAndRecordsFailures(t *testing.T) {
	var ids []uuid.UUID
	for i := 0; i < 12; i++ {
		ids = append(ids, uuid.New())
	}
	repo := newRepo(ids...)
	src := &fakeSource{fail: map[uuid.UUID]bool{ids[3]: true}}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	svc := NewService(repo, src, 30*time.Minute)
	svc.now = func() time.Time { return now }

	if n := svc.RefreshDue(context.Background()); n != 12 {
		t.Fatalf("attempted %d", n)
	}
	if len(repo.replaced) != 11 || repo.failed[ids[3]] != "connection refused" {
		t.Fatalf("replaced %d, failed %v", len(repo.replaced), repo.failed)
	}
	if _, ok := repo.replaced[ids[3]]; ok {
		t.Fatal("a failed read must keep the previous catalog")
	}
	if !repo.stale.Equal(now.Add(-30 * time.Minute)) {
		t.Fatalf("stale cutoff = %v", repo.stale)
	}
	if p := src.peak.Load(); p > 4 {
		t.Fatalf("ran %d refreshes at once, want at most 4", p)
	}
}
