package operationapp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

type fakeJobRepo struct {
	items       map[uuid.UUID]*jobdom.Job
	logs        map[uuid.UUID][]jobdom.JobLog
	claimNext   *jobdom.Job // when set, ClaimNext returns this job once
	claimCalled bool
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

func (r *fakeJobRepo) Complete(_ context.Context, _ uuid.UUID, _ jobdom.Status, _ json.RawMessage, _ *string) error {
	return nil
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
