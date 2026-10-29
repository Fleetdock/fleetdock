package operationapp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
)

type copyHook struct {
	restored, dropped []bool
}

func (h *copyHook) OnCopyRestored(_ context.Context, _ uuid.UUID, ok bool, _ *string) {
	h.restored = append(h.restored, ok)
}

func (h *copyHook) OnCopyDropped(_ context.Context, _ uuid.UUID, ok bool) {
	h.dropped = append(h.dropped, ok)
}

type recordingMover struct{ restores int }

func (m *recordingMover) OnBackupComplete(context.Context, *jobdom.Job, bool) {}
func (m *recordingMover) OnRestoreComplete(context.Context, *jobdom.Job, bool, json.RawMessage) {
	m.restores++
}

func TestBackupCopyJobsGoToTheCopyHook(t *testing.T) {
	repo := newFakeJobRepo()
	svc := NewService(repo, nil, nil, nil, nil, nil)
	hook := &copyHook{}
	mover := &recordingMover{}
	svc.SetBackupCopyHook(hook)
	svc.SetMover(mover)
	dropped := 0
	svc.SetDatabaseDropHook(func(context.Context, uuid.UUID, uuid.UUID) { dropped++ })
	notes := &recordingNotifier{}
	svc.SetNotifier(notes)

	cid := uuid.New()
	restore, _ := json.Marshal(Params{InstanceID: uuid.NewString(), Database: "fds_x", BackupCopyID: cid.String()})
	r := &jobdom.Job{ID: uuid.New(), Type: jobdom.TypeRestore, Status: jobdom.StatusRunning, Params: restore, ResourceID: &cid}
	repo.items[r.ID] = r
	if err := svc.Complete(context.Background(), r.ID, jobdom.StatusFailed, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(hook.restored) != 1 || hook.restored[0] {
		t.Fatalf("restored = %v", hook.restored)
	}
	if mover.restores != 0 || len(notes.events) != 0 {
		t.Fatalf("a copy restore must not reach the mover (%d) or raise restore.failed (%v)", mover.restores, notes.events)
	}

	// Dropping the copy's database must never touch the source database's
	// access roles, even if a DatabaseID slipped into the params.
	drop, _ := json.Marshal(Params{InstanceID: uuid.NewString(), Database: "fds_x", DatabaseID: uuid.NewString(), BackupCopyID: cid.String()})
	d := &jobdom.Job{ID: uuid.New(), Type: jobdom.TypeDeleteDatabase, Status: jobdom.StatusRunning, Params: drop}
	repo.items[d.ID] = d
	if err := svc.Complete(context.Background(), d.ID, jobdom.StatusSucceeded, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(hook.dropped) != 1 || !hook.dropped[0] || dropped != 0 {
		t.Fatalf("dropped = %v, role cleanup ran %d times", hook.dropped, dropped)
	}
}

type recordingNotifier struct{ events []string }

func (n *recordingNotifier) Emit(_ context.Context, event, _, _, _, _ string, _ uuid.UUID) {
	n.events = append(n.events, event)
}
