package backupcopyapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	backupdom "github.com/Fleetdock/fleetdock/backend/internal/domain/backup"
	backupcopydom "github.com/Fleetdock/fleetdock/backend/internal/domain/backupcopy"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

type memCopies struct{ items map[uuid.UUID]*backupcopydom.Copy }

func (m *memCopies) Create(_ context.Context, c *backupcopydom.Copy) error {
	for _, o := range m.items {
		if o.DatabaseID == c.DatabaseID && o.Live() {
			return apperr.Conflict("live copy")
		}
	}
	c.ID = uuid.New()
	cp := *c
	m.items[c.ID] = &cp
	return nil
}
func (m *memCopies) Get(_ context.Context, id uuid.UUID) (*backupcopydom.Copy, error) {
	c, ok := m.items[id]
	if !ok {
		return nil, apperr.NotFound("no")
	}
	cp := *c
	return &cp, nil
}
func (m *memCopies) ListByDatabase(context.Context, uuid.UUID) ([]backupcopydom.Copy, error) { return nil, nil }
func (m *memCopies) SetJob(_ context.Context, id, job uuid.UUID) error {
	m.items[id].JobID = &job
	return nil
}
func (m *memCopies) MarkReady(_ context.Context, id uuid.UUID, at, exp time.Time) error {
	m.items[id].Status, m.items[id].ReadyAt, m.items[id].ExpiresAt = backupcopydom.StatusReady, &at, &exp
	return nil
}
func (m *memCopies) MarkFailed(_ context.Context, id uuid.UUID, msg string) error {
	m.items[id].Status, m.items[id].Error = backupcopydom.StatusFailed, &msg
	return nil
}
func (m *memCopies) MarkDropping(_ context.Context, id uuid.UUID, job *uuid.UUID) error {
	m.items[id].Status = backupcopydom.StatusDropping
	if job != nil {
		m.items[id].DropJobID = job
		m.items[id].DropAttempts++
	}
	return nil
}
func (m *memCopies) MarkDropped(_ context.Context, id uuid.UUID) error {
	m.items[id].Status = backupcopydom.StatusDropped
	return nil
}
func (m *memCopies) SetExpiry(_ context.Context, id uuid.UUID, at time.Time) error {
	m.items[id].ExpiresAt = &at
	return nil
}
func (m *memCopies) ListDue(_ context.Context, now, _ time.Time) ([]backupcopydom.Copy, error) {
	var out []backupcopydom.Copy
	for _, c := range m.items {
		if (c.Status == backupcopydom.StatusReady && c.ExpiresAt.Before(now)) || (c.Status == backupcopydom.StatusFailed && c.DropJobID == nil) {
			out = append(out, *c)
		}
	}
	return out, nil
}

type oneBackup struct {
	backupdom.Repository
	b *backupdom.Backup
}

func (r oneBackup) GetByID(context.Context, uuid.UUID) (*backupdom.Backup, error) { return r.b, nil }

type oneDB struct {
	databasedom.Repository
	db *databasedom.Database
}

func (r oneDB) GetByID(context.Context, uuid.UUID) (*databasedom.Database, error) { return r.db, nil }

type fakeRestorer struct {
	restores, drops int
	failRestore     bool
}

func (f *fakeRestorer) RestoreCopy(context.Context, uuid.UUID, uuid.UUID, string, *uuid.UUID) (*jobdom.Job, error) {
	f.restores++
	if f.failRestore {
		return nil, errors.New("no storage")
	}
	return &jobdom.Job{ID: uuid.New()}, nil
}

func (f *fakeRestorer) DropCopy(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, *uuid.UUID) (*jobdom.Job, error) {
	f.drops++
	return &jobdom.Job{ID: uuid.New()}, nil
}

func fixture(t *testing.T) (*Service, *memCopies, *fakeRestorer, *databasedom.Database, *backupdom.Backup, *time.Time) {
	t.Helper()
	db := &databasedom.Database{ID: uuid.New(), InstanceID: uuid.New(), Name: "shop"}
	url, size := "s3://bucket/k", int64(1000)
	b := &backupdom.Backup{ID: uuid.New(), DatabaseID: db.ID, Status: backupdom.StatusCompleted, StorageURL: &url, SizeBytes: &size}
	copies := &memCopies{items: map[uuid.UUID]*backupcopydom.Copy{}}
	r := &fakeRestorer{}
	svc := NewService(copies, oneBackup{b: b}, oneDB{db: db}, r, time.Hour, 5000)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	return svc, copies, r, db, b, &now
}

func TestCreateValidates(t *testing.T) {
	ctx := context.Background()
	svc, _, r, db, b, _ := fixture(t)
	other := uuid.New()
	b.DatabaseID = other
	if _, err := svc.Create(ctx, db.ID, b.ID, nil); apperr.KindOf(err) != apperr.KindInvalid {
		t.Errorf("another database's backup: %v", err)
	}
	b.DatabaseID = db.ID
	big := int64(9000)
	b.SizeBytes = &big
	if _, err := svc.Create(ctx, db.ID, b.ID, nil); apperr.KindOf(err) != apperr.KindInvalid {
		t.Errorf("too big: %v", err)
	}
	b.SizeBytes = nil
	db.System = true
	if _, err := svc.Create(ctx, db.ID, b.ID, nil); apperr.KindOf(err) != apperr.KindInvalid {
		t.Errorf("system database: %v", err)
	}
	db.System = false
	c, err := svc.Create(ctx, db.ID, b.ID, nil)
	if err != nil || c.JobID == nil || !databasedom.IsScratchName(c.ScratchName) || c.ScratchName[:4] != "fds_" {
		t.Fatalf("create = %+v, %v", c, err)
	}
	if _, err := svc.Create(ctx, db.ID, b.ID, nil); apperr.KindOf(err) != apperr.KindConflict {
		t.Errorf("second live copy: %v", err)
	}
	if r.restores != 1 {
		t.Errorf("restores = %d", r.restores)
	}
}

func TestLifecycle(t *testing.T) {
	ctx := context.Background()
	svc, copies, r, db, b, now := fixture(t)
	c, _ := svc.Create(ctx, db.ID, b.ID, nil)

	svc.OnCopyRestored(ctx, c.ID, true, nil)
	got := copies.items[c.ID]
	if got.Status != backupcopydom.StatusReady || !got.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("after restore = %+v", got)
	}

	// Extending is capped at MaxTTL after the copy became ready.
	*now = now.Add(23*time.Hour + 30*time.Minute)
	ext, err := svc.Extend(ctx, db.ID, c.ID)
	if err != nil || !ext.ExpiresAt.Equal(got.ReadyAt.Add(backupcopydom.MaxTTL)) {
		t.Fatalf("extend = %v, %v", ext.ExpiresAt, err)
	}

	// Expired: the worker drops it.
	*now = now.Add(2 * time.Hour)
	if n, err := svc.ExpireDue(ctx); err != nil || n != 1 || r.drops != 1 {
		t.Fatalf("expire = %d, %v, drops %d", n, err, r.drops)
	}
	if copies.items[c.ID].Status != backupcopydom.StatusDropping {
		t.Fatalf("status = %s", copies.items[c.ID].Status)
	}
	svc.OnCopyDropped(ctx, c.ID, true)
	if copies.items[c.ID].Status != backupcopydom.StatusDropped {
		t.Fatalf("status = %s", copies.items[c.ID].Status)
	}
}

func TestFailedRestoreAndDiscardWhileRestoring(t *testing.T) {
	ctx := context.Background()
	svc, copies, r, db, b, _ := fixture(t)
	c, _ := svc.Create(ctx, db.ID, b.ID, nil)
	msg := "checksum mismatch"
	svc.OnCopyRestored(ctx, c.ID, false, &msg)
	if copies.items[c.ID].Status != backupcopydom.StatusDropping || *copies.items[c.ID].Error != msg || r.drops != 1 {
		t.Fatalf("failed restore = %+v, drops %d", copies.items[c.ID], r.drops)
	}

	c2, err := svc.Create(ctx, db.ID, b.ID, nil)
	if err != nil {
		t.Fatalf("a failed copy must not block a new one: %v", err)
	}
	if err := svc.Discard(ctx, db.ID, c2.ID, nil); err != nil {
		t.Fatal(err)
	}
	if copies.items[c2.ID].Status != backupcopydom.StatusDropping || r.drops != 1 {
		t.Fatal("discarding while restoring waits for the restore")
	}
	svc.OnCopyRestored(ctx, c2.ID, true, nil)
	if r.drops != 2 || copies.items[c2.ID].Status == backupcopydom.StatusReady {
		t.Fatalf("a discarded copy must be dropped when its restore ends, not become ready: %+v", copies.items[c2.ID])
	}
}

func TestRestoreStartFailureLeavesNothingLive(t *testing.T) {
	ctx := context.Background()
	svc, copies, r, db, b, _ := fixture(t)
	r.failRestore = true
	if _, err := svc.Create(ctx, db.ID, b.ID, nil); err == nil {
		t.Fatal("expected an error")
	}
	for _, c := range copies.items {
		if c.Live() {
			t.Fatalf("copy left live: %+v", c)
		}
	}
}
