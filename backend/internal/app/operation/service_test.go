package operationapp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	backupdom "github.com/Fleetdock/fleetdock/backend/internal/domain/backup"
	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

type fakeJobRepo struct {
	items       map[uuid.UUID]*jobdom.Job
	logs        map[uuid.UUID][]jobdom.JobLog
	claimNext   *jobdom.Job // when set, ClaimNext returns this job once
	claimCalled bool
	stuck       []uuid.UUID
	completed   map[uuid.UUID]jobdom.Status
}

func newFakeJobRepo() *fakeJobRepo {
	return &fakeJobRepo{
		items: map[uuid.UUID]*jobdom.Job{},
		logs:  map[uuid.UUID][]jobdom.JobLog{},
	}
}

func (r *fakeJobRepo) Create(_ context.Context, j *jobdom.Job) error {
	r.items[j.ID] = j
	return nil
}

func (r *fakeJobRepo) GetByID(_ context.Context, id uuid.UUID) (*jobdom.Job, error) {
	j, ok := r.items[id]
	if !ok {
		return nil, apperr.NotFound("job not found")
	}
	return j, nil
}

func (r *fakeJobRepo) List(_ context.Context, _ jobdom.ListFilter) (jobdom.Page, error) {
	items := make([]*jobdom.Job, 0, len(r.items))
	for _, j := range r.items {
		items = append(items, j)
	}
	return jobdom.Page{Items: items, Total: len(items)}, nil
}

func (r *fakeJobRepo) ClaimNext(_ context.Context, _ *uuid.UUID) (*jobdom.Job, error) {
	if r.claimNext != nil && !r.claimCalled {
		r.claimCalled = true
		j := r.claimNext
		j.Status = jobdom.StatusRunning
		return j, nil
	}
	return nil, nil
}

func (r *fakeJobRepo) Complete(_ context.Context, id uuid.UUID, st jobdom.Status, _ json.RawMessage, _ *string) error {
	if r.completed == nil {
		r.completed = map[uuid.UUID]jobdom.Status{}
	}
	r.completed[id] = st
	return nil
}

func (r *fakeJobRepo) ListStuck(_ context.Context, _ time.Time) ([]uuid.UUID, error) {
	return r.stuck, nil
}

func (r *fakeJobRepo) UpdateProgress(_ context.Context, _ uuid.UUID, _ int) error { return nil }

func (r *fakeJobRepo) AppendLogs(_ context.Context, id uuid.UUID, lines []jobdom.JobLog) error {
	r.logs[id] = append(r.logs[id], lines...)
	return nil
}

func (r *fakeJobRepo) ListLogs(_ context.Context, id uuid.UUID, afterSeq, limit int) ([]jobdom.JobLog, error) {
	all := r.logs[id]
	out := make([]jobdom.JobLog, 0)
	for _, line := range all {
		if line.Seq > afterSeq {
			out = append(out, line)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func TestCreate_PersistsPendingJob(t *testing.T) {
	repo := newFakeJobRepo()
	svc := NewService(repo, nil, nil, nil, nil, nil)

	dbID := uuid.New()
	job, err := svc.Create(context.Background(), jobdom.TypeCreateDatabase, "database", &dbID, nil, Params{
		InstanceID: uuid.NewString(),
		Database:   "app",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.Status != jobdom.StatusPending {
		t.Fatalf("expected pending job, got %s", job.Status)
	}
	if _, ok := repo.items[job.ID]; !ok {
		t.Fatal("expected job to be stored")
	}
}

func TestGet_InvalidID(t *testing.T) {
	svc := NewService(newFakeJobRepo(), nil, nil, nil, nil, nil)

	_, err := svc.Get(context.Background(), "bad")
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid, got %v", apperr.KindOf(err))
	}
}

func TestLogs_DefaultLimit(t *testing.T) {
	repo := newFakeJobRepo()
	id := uuid.New()
	repo.items[id] = &jobdom.Job{ID: id, Status: jobdom.StatusPending}
	repo.logs[id] = []jobdom.JobLog{{Seq: 1, Message: "hello"}}
	svc := NewService(repo, nil, nil, nil, nil, nil)

	lines, err := svc.Logs(context.Background(), id.String(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}
}

func TestList_ClampLimit(t *testing.T) {
	repo := newFakeJobRepo()
	svc := NewService(repo, nil, nil, nil, nil, nil)

	res, err := svc.List(context.Background(), ListParams{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	if res.Limit != 100 {
		t.Fatalf("expected limit clamped to 100, got %d", res.Limit)
	}
}

func TestBuildPayload_ReconcileGateway(t *testing.T) {
	svc := NewService(newFakeJobRepo(), nil, nil, nil, nil, nil)
	j := &jobdom.Job{
		ID:     uuid.New(),
		Type:   jobdom.TypeReconcileGateway,
		Params: json.RawMessage(`{}`),
	}
	payload, err := svc.buildPayload(context.Background(), j)
	if err != nil {
		t.Fatalf("expected no error for reconcile_gateway with empty params, got %v", err)
	}
	if payload == nil {
		t.Fatal("expected empty payload, got nil")
	}
}

func TestClaim_ReconcileGateway(t *testing.T) {
	repo := newFakeJobRepo()
	dbID := uuid.New()
	j := &jobdom.Job{
		ID:           uuid.New(),
		Type:         jobdom.TypeReconcileGateway,
		ResourceType: "database",
		ResourceID:   &dbID,
		Status:       jobdom.StatusPending,
		Params:       json.RawMessage(`{}`),
	}
	repo.claimNext = j
	// instances/databases/secrets are nil — claim must succeed without them
	svc := NewService(repo, nil, nil, nil, nil, nil)

	claimed, payload, err := svc.Claim(context.Background(), nil)
	if err != nil {
		t.Fatalf("claim reconcile_gateway: %v", err)
	}
	if claimed == nil || claimed.ID != j.ID {
		t.Fatal("expected claimed reconcile_gateway job")
	}
	if payload == nil {
		t.Fatal("expected empty payload")
	}
}

func TestFailStuck_FailsThroughComplete(t *testing.T) {
	repo := newFakeJobRepo()
	j := &jobdom.Job{ID: uuid.New(), Type: jobdom.TypeTestConnection, Status: jobdom.StatusRunning, Params: json.RawMessage(`{}`)}
	repo.items[j.ID] = j
	repo.stuck = []uuid.UUID{j.ID}
	svc := NewService(repo, nil, nil, nil, nil, nil)

	n, err := svc.FailStuck(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("FailStuck: %v", err)
	}
	if n != 1 || repo.completed[j.ID] != jobdom.StatusFailed {
		t.Fatalf("n = %d, completed = %v; want the stuck job failed", n, repo.completed)
	}
}

type verifyBackups struct {
	backupdom.Repository
	status string
}

func (b *verifyBackups) SetVerify(_ context.Context, _ uuid.UUID, status string, _ *string) error {
	b.status = status
	return nil
}

func TestCompleteVerification_RecordsAndDropsScratch(t *testing.T) {
	for _, ok := range []bool{true, false} {
		repo := newFakeJobRepo()
		backups := &verifyBackups{}
		svc := NewService(repo, nil, nil, backups, nil, nil)
		bid, iid := uuid.New(), uuid.New()
		params, _ := json.Marshal(Params{InstanceID: iid.String(), Database: "fdv_x", VerifyBackupID: bid.String()})
		j := &jobdom.Job{ID: uuid.New(), Type: jobdom.TypeRestore, Status: jobdom.StatusRunning, Params: params}
		repo.items[j.ID] = j

		status := jobdom.StatusSucceeded
		if !ok {
			status = jobdom.StatusFailed
		}
		if err := svc.Complete(context.Background(), j.ID, status, nil, nil); err != nil {
			t.Fatal(err)
		}
		want := map[bool]string{true: "passed", false: "failed"}[ok]
		if backups.status != want {
			t.Errorf("ok=%v: verify status = %q, want %q", ok, backups.status, want)
		}
		var drop *jobdom.Job
		for _, other := range repo.items {
			if other.Type == jobdom.TypeDeleteDatabase {
				drop = other
			}
		}
		if drop == nil {
			t.Fatalf("ok=%v: no drop job for the scratch database", ok)
		}
		var dp Params
		_ = json.Unmarshal(drop.Params, &dp)
		if dp.Database != "fdv_x" || dp.InstanceID != iid.String() || dp.DatabaseID != "" {
			t.Errorf("drop params = %+v; must target only the scratch database", dp)
		}
	}
}
