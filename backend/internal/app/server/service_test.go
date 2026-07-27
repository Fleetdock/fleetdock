package serverapp

import (
	"context"
	"testing"

	"github.com/google/uuid"

	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// fakeRepo is an in-memory Repository used to test the service in isolation.
type fakeRepo struct {
	items     map[uuid.UUID]*serverdom.Server
	names     map[string]bool
	instances map[uuid.UUID]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		items:     map[uuid.UUID]*serverdom.Server{},
		names:     map[string]bool{},
		instances: map[uuid.UUID]bool{},
	}
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
	if !ok || s.DeletedAt != nil {
		return nil, apperr.NotFound("server not found")
	}
	return s, nil
}

func (r *fakeRepo) List(_ context.Context, f serverdom.ListFilter) (serverdom.Page, error) {
	items := make([]*serverdom.Server, 0)
	for _, s := range r.items {
		if s.DeletedAt != nil {
			continue
		}
		if f.Status != nil && s.Status != *f.Status {
			continue
		}
		items = append(items, s)
	}
	return serverdom.Page{Items: items, Total: len(items)}, nil
}

func (r *fakeRepo) Update(_ context.Context, s *serverdom.Server) error {
	cur, ok := r.items[s.ID]
	if !ok || cur.DeletedAt != nil {
		return apperr.NotFound("server not found")
	}
	if cur.Name != s.Name {
		if r.names[s.Name] {
			return apperr.Conflict("server name already exists")
		}
		delete(r.names, cur.Name)
		r.names[s.Name] = true
	}
	r.items[s.ID] = s
	return nil
}

func (r *fakeRepo) SoftDelete(_ context.Context, id uuid.UUID) error {
	s, ok := r.items[id]
	if !ok || s.DeletedAt != nil {
		return apperr.NotFound("server not found")
	}
	now := s.UpdatedAt
	s.DeletedAt = &now
	delete(r.names, s.Name)
	return nil
}

func (r *fakeRepo) HasActiveInstances(_ context.Context, id uuid.UUID) (bool, error) {
	return r.instances[id], nil
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

func TestUpdate_Rename(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	got, err := svc.Register(context.Background(), RegisterInput{Name: "db-1", Hostname: "db1.internal"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	updated, err := svc.Update(context.Background(), UpdateInput{ID: got.ID.String(), Name: ptr("db-2")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "db-2" {
		t.Errorf("expected db-2, got %s", updated.Name)
	}
}

func TestDelete_BlockedWithInstances(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	got, err := svc.Register(context.Background(), RegisterInput{Name: "db-1", Hostname: "db1.internal"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	repo.instances[got.ID] = true
	if err := svc.Delete(context.Background(), got.ID.String()); apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("expected conflict, got %v", apperr.KindOf(err))
	}
}

func TestDelete_Success(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	got, err := svc.Register(context.Background(), RegisterInput{Name: "db-1", Hostname: "db1.internal"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := svc.Delete(context.Background(), got.ID.String()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(context.Background(), got.ID.String()); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("expected not found after delete, got %v", apperr.KindOf(err))
	}
}

func ptr[T any](v T) *T { return &v }
