package tableviewapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	tableviewdom "github.com/Fleetdock/fleetdock/backend/internal/domain/tableview"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

type memRepo struct{ views map[uuid.UUID]*tableviewdom.View }

func (m *memRepo) List(_ context.Context, db uuid.UUID, table string, user uuid.UUID) ([]tableviewdom.View, error) {
	var out []tableviewdom.View
	for _, v := range m.views {
		if v.DatabaseID == db && (table == "" || v.Table == table) && (v.Shared || *v.CreatedBy == user) {
			out = append(out, *v)
		}
	}
	return out, nil
}
func (m *memRepo) Get(_ context.Context, db, id uuid.UUID) (*tableviewdom.View, error) {
	v, ok := m.views[id]
	if !ok || v.DatabaseID != db {
		return nil, apperr.NotFound("view not found")
	}
	c := *v
	return &c, nil
}
func (m *memRepo) Create(_ context.Context, v *tableviewdom.View) error {
	v.ID = uuid.New()
	c := *v
	m.views[v.ID] = &c
	return nil
}
func (m *memRepo) Update(_ context.Context, v *tableviewdom.View) error {
	c := *v
	m.views[v.ID] = &c
	return nil
}
func (m *memRepo) Delete(_ context.Context, _, id uuid.UUID) error {
	delete(m.views, id)
	return nil
}

func ptr[T any](v T) *T { return &v }

func kind(err error) apperr.Kind {
	if e, ok := err.(*apperr.Error); ok {
		return e.Kind
	}
	return -1
}

func TestTableViews(t *testing.T) {
	ctx := context.Background()
	repo := &memRepo{views: map[uuid.UUID]*tableviewdom.View{}}
	s := NewService(repo)
	db := uuid.NewString()
	ann, bob := uuid.New(), uuid.New()
	state := json.RawMessage(`{"filters":[],"sort":[]}`)

	for _, in := range []Input{
		{Table: ptr("orders"), Name: ptr("  "), State: &state},
		{Table: ptr(""), Name: ptr("x"), State: &state},
		{Table: ptr("orders"), Name: ptr(strings.Repeat("n", 101)), State: &state},
		{Table: ptr("orders"), Name: ptr("x"), State: ptr(json.RawMessage(`[1]`))},
		{Table: ptr("orders"), Name: ptr("x"), State: ptr(json.RawMessage(`{"a":"` + strings.Repeat("x", MaxStateBytes) + `"}`))},
		{Table: ptr("orders"), Name: ptr("x")},
	} {
		if _, err := s.Create(ctx, db, ann, in); kind(err) != apperr.KindInvalid {
			t.Errorf("create %+v: err = %v", in, err)
		}
	}

	private, err := s.Create(ctx, db, ann, Input{Table: ptr("orders"), Name: ptr(" Mine "), State: &state})
	if err != nil || private.Name != "Mine" || private.Shared {
		t.Fatalf("create = %+v, %v", private, err)
	}
	shared, _ := s.Create(ctx, db, ann, Input{Table: ptr("orders"), Name: ptr("Team"), State: &state, Shared: ptr(true)})

	if l, _ := s.List(ctx, db, "orders", bob); len(l) != 1 || l[0].ID != shared.ID {
		t.Errorf("bob sees %+v", l)
	}
	if l, _ := s.List(ctx, db, "", ann); len(l) != 2 {
		t.Errorf("ann sees %d", len(l))
	}

	// Others cannot see private views, nor change shared ones.
	if _, err := s.Update(ctx, db, private.ID.String(), bob, Input{Name: ptr("x")}); kind(err) != apperr.KindNotFound {
		t.Errorf("bob updates private: %v", err)
	}
	if _, err := s.Update(ctx, db, shared.ID.String(), bob, Input{Name: ptr("x")}); kind(err) != apperr.KindForbidden {
		t.Errorf("bob updates shared: %v", err)
	}
	if err := s.Delete(ctx, db, shared.ID.String(), bob, false); kind(err) != apperr.KindForbidden {
		t.Errorf("bob deletes shared without write: %v", err)
	}
	if err := s.Delete(ctx, db, private.ID.String(), bob, true); kind(err) != apperr.KindNotFound {
		t.Errorf("bob deletes private with write: %v", err)
	}

	up, err := s.Update(ctx, db, private.ID.String(), ann, Input{Shared: ptr(true)})
	if err != nil || !up.Shared || up.Name != "Mine" {
		t.Errorf("ann shares = %+v, %v", up, err)
	}
	if err := s.Delete(ctx, db, shared.ID.String(), bob, true); err != nil {
		t.Errorf("bob deletes shared with write: %v", err)
	}
	if err := s.Delete(ctx, db, private.ID.String(), ann, false); err != nil {
		t.Errorf("ann deletes her own: %v", err)
	}
}
