package operationapp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	backupdom "github.com/Fleetdock/fleetdock/backend/internal/domain/backup"
	backupdestdom "github.com/Fleetdock/fleetdock/backend/internal/domain/backupdest"
	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

type recordingBackups struct {
	backupdom.Repository
	completed map[uuid.UUID]backupdom.CompleteInput
	expired   []backupdom.Expired
	marked    []uuid.UUID
}

func (r *recordingBackups) Complete(_ context.Context, id uuid.UUID, in backupdom.CompleteInput) error {
	r.completed[id] = in
	return nil
}
func (r *recordingBackups) ListExpired(context.Context, time.Time, int) ([]backupdom.Expired, error) {
	return r.expired, nil
}
func (r *recordingBackups) MarkExpired(_ context.Context, id uuid.UUID) error {
	r.marked = append(r.marked, id)
	return nil
}

type event struct{ kind, severity string }

type recordingEmitter struct{ events []event }

func (e *recordingEmitter) Emit(_ context.Context, kind, _, _, severity, _ string, _ uuid.UUID) {
	e.events = append(e.events, event{kind, severity})
}

func TestBackupJobCompletion(t *testing.T) {
	ctx := context.Background()
	for _, ok := range []bool{true, false} {
		jobs := newFakeJobRepo()
		backups := &recordingBackups{completed: map[uuid.UUID]backupdom.CompleteInput{}}
		emitter := &recordingEmitter{}
		svc := NewService(jobs, nil, nil, backups, nil, nil)
		svc.SetNotifier(emitter)

		bid := uuid.New()
		params, _ := json.Marshal(Params{BackupID: bid.String()})
		j := &jobdom.Job{ID: uuid.New(), Type: jobdom.TypeBackup, Status: jobdom.StatusRunning, Params: params}
		jobs.items[j.ID] = j

		status, errMsg := jobdom.StatusSucceeded, (*string)(nil)
		result := json.RawMessage(`{"ok":true,"size_bytes":2048,"checksum":"sha256:abc","storage_url":"s3://b/k.sql.gz"}`)
		if !ok {
			msg := "mariadb-dump: Access denied"
			status, errMsg, result = jobdom.StatusFailed, &msg, nil
		}
		if err := svc.Complete(ctx, j.ID, status, result, errMsg); err != nil {
			t.Fatal(err)
		}

		got, done := backups.completed[bid]
		if !done {
			t.Fatalf("ok=%v: the backup record was not completed", ok)
		}
		if ok {
			if got.Status != backupdom.StatusCompleted || got.SizeBytes == nil || *got.SizeBytes != 2048 ||
				got.Checksum == nil || *got.Checksum != "sha256:abc" || got.StorageURL == nil || *got.StorageURL != "s3://b/k.sql.gz" {
				t.Errorf("completed backup = %+v", got)
			}
			if len(emitter.events) != 0 {
				t.Errorf("a successful backup raised %v", emitter.events)
			}
		} else {
			if got.Status != backupdom.StatusFailed || got.Error == nil || *got.Error != "mariadb-dump: Access denied" {
				t.Errorf("failed backup = %+v", got)
			}
			if len(emitter.events) != 1 || emitter.events[0] != (event{"backup.failed", "critical"}) {
				t.Errorf("events = %v, want one critical backup.failed", emitter.events)
			}
		}
	}
}

type noDests struct{ backupdestdom.Repository }

func (noDests) GetByID(context.Context, uuid.UUID) (*backupdestdom.Destination, error) {
	return nil, apperr.NotFound("destination not found")
}

// A backup whose storage is gone stays listed for the next run instead of
// being marked expired without its object deleted.
func TestPruneSkipsBackupsItCannotDelete(t *testing.T) {
	backups := &recordingBackups{expired: []backupdom.Expired{{ID: uuid.New(), DestinationID: uuid.New(), StorageURL: "s3://b/k"}}}
	svc := NewService(newFakeJobRepo(), nil, nil, backups, noDests{}, nil)
	n, err := svc.PruneExpiredBackups(context.Background(), 10)
	if err != nil || n != 0 || len(backups.marked) != 0 {
		t.Fatalf("pruned %d, marked %v, err %v", n, backups.marked, err)
	}
}

func TestKeyFromStorageURL(t *testing.T) {
	if k, err := keyFromStorageURL("s3://backups/prod/backups/db/1.sql.gz", "backups"); err != nil || k != "prod/backups/db/1.sql.gz" {
		t.Errorf("key = %q, %v", k, err)
	}
	for _, u := range []string{"s3://other/k", "s3://backups-evil/k", "https://backups/k"} {
		if _, err := keyFromStorageURL(u, "backups"); err == nil {
			t.Errorf("%s accepted for bucket backups", u)
		}
	}
}
