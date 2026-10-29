package schemahistoryapp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	schemahistorydom "github.com/Fleetdock/fleetdock/backend/internal/domain/schemahistory"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/schemadiff"
)

type fakeRepo struct {
	watches map[uuid.UUID]*schemahistorydom.Watch
	snaps   []schemahistorydom.Snapshot
	checked map[uuid.UUID]string
}

func newRepo() *fakeRepo {
	return &fakeRepo{watches: map[uuid.UUID]*schemahistorydom.Watch{}, checked: map[uuid.UUID]string{}}
}

func (r *fakeRepo) GetWatch(_ context.Context, id uuid.UUID) (*schemahistorydom.Watch, error) {
	return r.watches[id], nil
}

func (r *fakeRepo) SetWatch(_ context.Context, id uuid.UUID, enabled bool, _ *uuid.UUID) (*schemahistorydom.Watch, error) {
	w := &schemahistorydom.Watch{DatabaseID: id, Enabled: enabled}
	r.watches[id] = w
	return w, nil
}

func (r *fakeRepo) ListDue(context.Context, time.Time, int) ([]uuid.UUID, error) {
	var out []uuid.UUID
	for id, w := range r.watches {
		if w.Enabled {
			out = append(out, id)
		}
	}
	return out, nil
}

func (r *fakeRepo) MarkChecked(_ context.Context, id uuid.UUID, _ time.Time, msg string) error {
	r.checked[id] = msg
	return nil
}

func (r *fakeRepo) Latest(_ context.Context, id uuid.UUID) (*schemahistorydom.Snapshot, error) {
	for i := len(r.snaps) - 1; i >= 0; i-- {
		if r.snaps[i].DatabaseID == id {
			s := r.snaps[i]
			return &s, nil
		}
	}
	return nil, nil
}

func (r *fakeRepo) Add(_ context.Context, s *schemahistorydom.Snapshot) error {
	s.ID = uuid.New()
	s.TakenAt = time.Now()
	r.snaps = append(r.snaps, *s)
	return nil
}

func (r *fakeRepo) List(_ context.Context, id uuid.UUID, _ int) ([]schemahistorydom.Snapshot, error) {
	var out []schemahistorydom.Snapshot
	for _, s := range r.snaps {
		if s.DatabaseID == id {
			out = append([]schemahistorydom.Snapshot{s}, out...)
		}
	}
	return out, nil
}

func (r *fakeRepo) Get(_ context.Context, db, id uuid.UUID) (*schemahistorydom.Snapshot, error) {
	for _, s := range r.snaps {
		if s.ID == id && s.DatabaseID == db {
			return &s, nil
		}
	}
	return nil, apperr.NotFound("schema snapshot not found")
}

type fakeSource struct {
	schemas map[uuid.UUID]*schemadiff.Schema
}

func (f *fakeSource) Schema(_ context.Context, id string) (*schemadiff.Schema, error) {
	return f.schemas[uuid.MustParse(id)], nil
}

func (f *fakeSource) AdminSchema(_ context.Context, id uuid.UUID) (*schemadiff.Schema, error) {
	return f.schemas[id], nil
}

type fakeDBs struct{}

func (fakeDBs) GetByID(_ context.Context, id uuid.UUID) (*databasedom.Database, error) {
	return &databasedom.Database{ID: id, Name: "db_" + id.String()[:4]}, nil
}

type event struct{ kind, title, msg string }

type fakeNotifier struct{ events []event }

func (n *fakeNotifier) Emit(_ context.Context, kind, title, msg, _ string, _ string, _ uuid.UUID) {
	n.events = append(n.events, event{kind, title, msg})
}

func schema(engine string, cols ...string) *schemadiff.Schema {
	t := schemadiff.Table{Name: "t"}
	for _, c := range cols {
		t.Columns = append(t.Columns, schemadiff.Column{Name: c, Def: "`" + c + "` int"})
	}
	s := &schemadiff.Schema{Dialect: schemadiff.MySQL, Engine: engine, Tables: []schemadiff.Table{t}}
	s.Canonicalize()
	return s
}

func TestSnapshotsOnlyOnChangeAndNotifies(t *testing.T) {
	ctx := context.Background()
	db := uuid.New()
	repo, src, n := newRepo(), &fakeSource{schemas: map[uuid.UUID]*schemadiff.Schema{db: schema("mysql", "a")}}, &fakeNotifier{}
	svc := NewService(repo, src, fakeDBs{}, time.Minute)
	svc.SetNotifier(n)

	first, created, err := svc.TakeSnapshot(ctx, db, schemahistorydom.TriggerManual, nil)
	if err != nil || !created || first.PreviousID != nil || !first.Summary.Stored {
		t.Fatalf("first = %+v, %v, %v", first, created, err)
	}
	if _, created, _ := svc.TakeSnapshot(ctx, db, schemahistorydom.TriggerScheduled, nil); created {
		t.Fatal("an unchanged schema made a new snapshot")
	}
	if len(n.events) != 0 {
		t.Fatal("the first snapshot notified")
	}

	src.schemas[db] = schema("mysql", "a", "b")
	second, created, err := svc.TakeSnapshot(ctx, db, schemahistorydom.TriggerScheduled, nil)
	if err != nil || !created || *second.PreviousID != first.ID {
		t.Fatalf("second = %+v, %v, %v", second, created, err)
	}
	if second.Summary.Changes.Added != 1 || second.Summary.Highlights[0] != "+ column t.b" {
		t.Fatalf("summary = %+v", second.Summary)
	}
	if len(n.events) != 1 || n.events[0].kind != EventSchemaChanged || !strings.Contains(n.events[0].msg, "column t.b") {
		t.Fatalf("events = %+v", n.events)
	}

	d, err := svc.Get(ctx, db, second.ID)
	if err != nil || d.Previous == nil || len(d.Changes) != 1 || d.Changes[0].Op != schemadiff.Added {
		t.Fatalf("detail = %+v, %v", d, err)
	}
}

func TestWatchTakesFirstSnapshotAndRunDue(t *testing.T) {
	ctx := context.Background()
	db := uuid.New()
	repo := newRepo()
	svc := NewService(repo, &fakeSource{schemas: map[uuid.UUID]*schemadiff.Schema{db: schema("mysql", "a")}}, fakeDBs{}, time.Minute)
	if _, err := svc.SetWatch(ctx, db, true, nil); err != nil {
		t.Fatal(err)
	}
	if len(repo.snaps) != 1 {
		t.Fatalf("snapshots after watching = %d", len(repo.snaps))
	}
	if n := svc.RunDue(ctx); n != 1 || len(repo.snaps) != 1 {
		t.Fatalf("RunDue = %d, snapshots %d", n, len(repo.snaps))
	}
	if msg, ok := repo.checked[db]; !ok || msg != "" {
		t.Fatalf("check not recorded: %q %v", msg, ok)
	}
	if NewService(repo, nil, fakeDBs{}, 0).RunDue(ctx) != 0 {
		t.Fatal("RunDue ran with checks disabled")
	}
}

func TestCompare(t *testing.T) {
	ctx := context.Background()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	src := &fakeSource{schemas: map[uuid.UUID]*schemadiff.Schema{
		a: schema("mysql", "x", "y"), b: schema("mysql", "x"), c: schema("mariadb", "x"),
	}}
	svc := NewService(newRepo(), src, fakeDBs{}, 0)
	out, err := svc.Compare(ctx, CompareInput{SourceDatabaseID: a, TargetDatabaseID: b})
	if err != nil || out.Same || len(out.Changes) != 1 || !strings.Contains(out.SQL, "ALTER TABLE `t` ADD COLUMN `y` int AFTER `x`;") {
		t.Fatalf("compare = %+v, %v", out, err)
	}
	if !strings.HasPrefix(out.SQL, "-- Makes db_"+b.String()[:4]+" match db_"+a.String()[:4]) {
		t.Fatalf("header: %s", out.SQL)
	}
	same, err := svc.Compare(ctx, CompareInput{SourceDatabaseID: b, TargetDatabaseID: b})
	if err != nil || !same.Same || same.SQL != "" || same.Changes == nil {
		t.Fatalf("same = %+v, %v", same, err)
	}
	if _, err := svc.Compare(ctx, CompareInput{SourceDatabaseID: a, TargetDatabaseID: c}); apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("mysql vs mariadb: %v", err)
	}
}
