package serverapp

import (
	"context"
	"testing"

	"github.com/google/uuid"

	serverdom "github.com/TajBrains/db-manager/backend/internal/domain/server"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

// fakeRepo is an in-memory Repository used to test the service in isolation.
type fakeRepo struct {
	items map[uuid.UUID]*serverdom.Server
	names map[string]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[uuid.UUID]*serverdom.Server{}, names: map[string]bool{}}
}

func (r *fakeRepo) Create(_ context.Context, s *serverdom.Server) error {
	if r.names[s.Name] {
		return apperr.Conflict("server name already exists")
	}
	r.names[s.Name] = true
	r.items[s.ID] = s
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*serverdom.Server, error) {
	s, ok := r.items[id]
	if !ok {
		return nil, apperr.NotFound("server not found")
	}
	return s, nil
}

func (r *fakeRepo) List(_ context.Context, f serverdom.ListFilter) (serverdom.Page, error) {
	items := make([]*serverdom.Server, 0)
	for _, s := range r.items {
		if f.Status != nil && s.Status != *f.Status {
			continue
		}
		items = append(items, s)
	}
	return serverdom.Page{Items: items, Total: len(items)}, nil
}

func TestRegister_Success(t *testing.T) {
	svc := NewService(newFakeRepo())
	got, err := svc.Register(context.Background(), RegisterInput{Name: "db-1", Hostname: "db1.internal"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != serverdom.StatusPending {
		t.Errorf("expected pending status, got %s", got.Status)
	}
	if got.ID == uuid.Nil {
		t.Error("expected a generated id")
	}
}

func TestRegister_InvalidName(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, err := svc.Register(context.Background(), RegisterInput{Name: "A", Hostname: "h"})
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid kind, got %v (%v)", apperr.KindOf(err), err)
	}
}

func TestRegister_Conflict(t *testing.T) {
	svc := NewService(newFakeRepo())
	in := RegisterInput{Name: "db-1", Hostname: "h"}
	if _, err := svc.Register(context.Background(), in); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if _, err := svc.Register(context.Background(), in); apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("expected conflict, got %v", apperr.KindOf(err))
	}
}

func TestGet_InvalidUUID(t *testing.T) {
	svc := NewService(newFakeRepo())
	if _, err := svc.Get(context.Background(), "not-a-uuid"); apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid, got %v", apperr.KindOf(err))
	}
}

func TestGet_NotFound(t *testing.T) {
	svc := NewService(newFakeRepo())
	if _, err := svc.Get(context.Background(), uuid.NewString()); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("expected not found, got %v", apperr.KindOf(err))
	}
}

func TestList_DefaultLimitAndStatusValidation(t *testing.T) {
	svc := NewService(newFakeRepo())

	res, err := svc.List(context.Background(), ListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Limit != defaultLimit {
		t.Errorf("expected default limit %d, got %d", defaultLimit, res.Limit)
	}

	if _, err := svc.List(context.Background(), ListParams{Status: "bogus"}); apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid status, got %v", apperr.KindOf(err))
	}
}
