package databaseapp

import (
	"context"
	"testing"

	"github.com/google/uuid"

	databasedom "github.com/TajBrains/db-manager/backend/internal/domain/database"
	instancedom "github.com/TajBrains/db-manager/backend/internal/domain/instance"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

type fakeDatabaseRepo struct {
	items map[uuid.UUID]*databasedom.Database
}

func newFakeDatabaseRepo() *fakeDatabaseRepo {
	return &fakeDatabaseRepo{items: map[uuid.UUID]*databasedom.Database{}}
}

func (r *fakeDatabaseRepo) Create(_ context.Context, db *databasedom.Database) error {
	r.items[db.ID] = db
	return nil
}

func (r *fakeDatabaseRepo) GetByID(_ context.Context, id uuid.UUID) (*databasedom.Database, error) {
	db, ok := r.items[id]
	if !ok {
		return nil, apperr.NotFound("database not found")
	}
	return db, nil
}

func (r *fakeDatabaseRepo) List(_ context.Context, _ databasedom.ListFilter) (databasedom.Page, error) {
	items := make([]*databasedom.Database, 0, len(r.items))
	for _, db := range r.items {
		items = append(items, db)
	}
	return databasedom.Page{Items: items, Total: len(items)}, nil
}

func (r *fakeDatabaseRepo) Lock(_ context.Context, id uuid.UUID, lockedBy uuid.UUID) (*databasedom.Database, error) {
	db, err := r.GetByID(context.Background(), id)
	if err != nil {
		return nil, err
	}
	db.LockedBy = &lockedBy
	return db, nil
}

func (r *fakeDatabaseRepo) Unlock(_ context.Context, id uuid.UUID) (*databasedom.Database, error) {
	db, err := r.GetByID(context.Background(), id)
	if err != nil {
		return nil, err
	}
	db.LockedBy = nil
	return db, nil
}

func (r *fakeDatabaseRepo) SoftDelete(_ context.Context, id uuid.UUID) error {
	delete(r.items, id)
	return nil
}

func (r *fakeDatabaseRepo) SetStatus(_ context.Context, _ uuid.UUID, _ databasedom.Status) error {
	return nil
}

var _ instancedom.Repository = (*fakeInstanceRepo)(nil)

type fakeInstanceRepo struct {
	items map[uuid.UUID]*instancedom.Instance
}

func (r *fakeInstanceRepo) Create(_ context.Context, _ *instancedom.Instance) error { return nil }
func (r *fakeInstanceRepo) GetByID(_ context.Context, id uuid.UUID) (*instancedom.Instance, error) {
	inst, ok := r.items[id]
	if !ok {
		return nil, apperr.NotFound("instance not found")
	}
	return inst, nil
}

func (r *fakeInstanceRepo) List(_ context.Context, _ instancedom.ListFilter) (instancedom.Page, error) {
	return instancedom.Page{}, nil
}
func (r *fakeInstanceRepo) SetRootSecretRef(_ context.Context, _ uuid.UUID, _ string) error { return nil }
func (r *fakeInstanceRepo) SetStatus(_ context.Context, _ uuid.UUID, _ instancedom.Status) error {
	return nil
}
func (r *fakeInstanceRepo) SetContainerID(_ context.Context, _ uuid.UUID, _ string) error { return nil }
func (r *fakeInstanceRepo) SoftDelete(_ context.Context, _ uuid.UUID) error               { return nil }

func TestCreate_InvalidInstanceID(t *testing.T) {
	svc := NewService(newFakeDatabaseRepo(), &fakeInstanceRepo{}, nil)

	_, err := svc.Create(context.Background(), CreateInput{
		InstanceID: "bad",
		Name:       "app_db",
	}, nil)
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid instance_id, got %v", apperr.KindOf(err))
	}
}

func TestCreate_MetadataOnly(t *testing.T) {
	iid := uuid.New()
	repo := newFakeDatabaseRepo()
	instances := &fakeInstanceRepo{
		items: map[uuid.UUID]*instancedom.Instance{
			iid: {ID: iid, Kind: instancedom.KindExternal},
		},
	}
	svc := NewService(repo, instances, nil)

	db, err := svc.Create(context.Background(), CreateInput{
		InstanceID: iid.String(),
		Name:       "app_db",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db.Status != databasedom.StatusActive {
		t.Fatalf("expected active metadata-only db, got %s", db.Status)
	}
}

func TestList_InvalidStatus(t *testing.T) {
	svc := NewService(newFakeDatabaseRepo(), &fakeInstanceRepo{}, nil)

	_, err := svc.List(context.Background(), ListParams{Status: "bogus"})
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid status, got %v", apperr.KindOf(err))
	}
}

func TestLockUnlock(t *testing.T) {
	repo := newFakeDatabaseRepo()
	id := uuid.New()
	iid := uuid.New()
	repo.items[id] = &databasedom.Database{ID: id, InstanceID: iid, Name: "app", Status: databasedom.StatusActive}
	svc := NewService(repo, &fakeInstanceRepo{}, nil)

	uid := uuid.New()
	locked, err := svc.Lock(context.Background(), id.String(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if locked.LockedBy == nil || *locked.LockedBy != uid {
		t.Fatal("expected lock to be set")
	}

	unlocked, err := svc.Unlock(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if unlocked.LockedBy != nil {
		t.Fatal("expected lock to be cleared")
	}
}

func TestDelete_RequiresCredentialsForPhysicalDrop(t *testing.T) {
	iid := uuid.New()
	dbID := uuid.New()
	repo := newFakeDatabaseRepo()
	repo.items[dbID] = &databasedom.Database{ID: dbID, InstanceID: iid, Name: "app", Status: databasedom.StatusActive}
	instances := &fakeInstanceRepo{
		items: map[uuid.UUID]*instancedom.Instance{
			iid: {ID: iid, Kind: instancedom.KindExternal},
		},
	}
	svc := NewService(repo, instances, nil)

	err := svc.Delete(context.Background(), dbID.String(), true, nil)
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid drop without credentials, got %v", apperr.KindOf(err))
	}
}
