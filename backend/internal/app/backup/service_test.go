package backupapp

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	backupdom "github.com/TajBrains/db-manager/backend/internal/domain/backup"
	instancedom "github.com/TajBrains/db-manager/backend/internal/domain/instance"
	"github.com/TajBrains/db-manager/backend/internal/platform/apperr"
)

func TestTrigger_InvalidUUIDs(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil)

	_, _, err := svc.Trigger(context.Background(), TriggerInput{
		DatabaseID:    "not-a-uuid",
		DestinationID: uuid.NewString(),
	})
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid database_id, got %v", apperr.KindOf(err))
	}

	_, _, err = svc.Trigger(context.Background(), TriggerInput{
		DatabaseID:    uuid.NewString(),
		DestinationID: "bad",
	})
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid destination_id, got %v", apperr.KindOf(err))
	}
}

func TestList_InvalidDatabaseFilter(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil)

	_, err := svc.List(context.Background(), ListParams{DatabaseID: "nope"})
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid, got %v", apperr.KindOf(err))
	}
}

func TestGet_InvalidID(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil)

	_, err := svc.Get(context.Background(), "bad-id")
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid, got %v", apperr.KindOf(err))
	}
}

func TestRestore_IncompleteBackup(t *testing.T) {
	bid := uuid.New()
	repo := &fakeBackupRepo{
		items: map[uuid.UUID]*backupdom.Backup{
			bid: {ID: bid, Status: backupdom.StatusPending},
		},
	}
	svc := NewService(repo, nil, nil, nil, nil)

	_, err := svc.Restore(context.Background(), RestoreInput{BackupID: bid.String()})
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid backup, got %v", apperr.KindOf(err))
	}
}

func TestBackupKey(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	got := backupKey("prod", "mydb", id)
	want := "prod/backups/mydb/11111111-1111-1111-1111-111111111111.sql.gz"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExecutorFor(t *testing.T) {
	sid := uuid.New()
	managed := &instancedom.Instance{Kind: instancedom.KindManaged, ServerID: &sid}
	if got := executorFor(managed); got == nil || *got != sid {
		t.Fatal("expected managed instance to route to server agent")
	}
	external := &instancedom.Instance{Kind: instancedom.KindExternal}
	if got := executorFor(external); got != nil {
		t.Fatal("expected external instance to run on control plane")
	}
}

type fakeBackupRepo struct {
	items map[uuid.UUID]*backupdom.Backup
}

func (r *fakeBackupRepo) Create(_ context.Context, _ *backupdom.Backup) error { return nil }
func (r *fakeBackupRepo) GetByID(_ context.Context, id uuid.UUID) (*backupdom.Backup, error) {
	b, ok := r.items[id]
	if !ok {
		return nil, apperr.NotFound("backup not found")
	}
	return b, nil
}
func (r *fakeBackupRepo) List(_ context.Context, _ backupdom.ListFilter) (backupdom.Page, error) {
	return backupdom.Page{}, nil
}
func (r *fakeBackupRepo) MarkRunning(_ context.Context, _ uuid.UUID) error { return nil }
func (r *fakeBackupRepo) Complete(_ context.Context, _ uuid.UUID, _ backupdom.CompleteInput) error {
	return nil
}
func (r *fakeBackupRepo) ListExpired(_ context.Context, _ time.Time, _ int) ([]backupdom.Expired, error) {
	return nil, nil
}
func (r *fakeBackupRepo) MarkExpired(_ context.Context, _ uuid.UUID) error { return nil }
func (r *fakeBackupRepo) CountByStatusSince(_ context.Context, _ time.Time) (map[backupdom.Status]int, error) {
	return nil, nil
}