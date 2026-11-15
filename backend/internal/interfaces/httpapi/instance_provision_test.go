package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	authapp "github.com/Fleetdock/fleetdock/backend/internal/app/auth"
	instanceapp "github.com/Fleetdock/fleetdock/backend/internal/app/instance"
	operationapp "github.com/Fleetdock/fleetdock/backend/internal/app/operation"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
)

// memJobs records created jobs; the provision handler only creates one.
type memJobs struct {
	jobdom.Repository
	created []*jobdom.Job
}

func (r *memJobs) Create(_ context.Context, j *jobdom.Job) error {
	r.created = append(r.created, j)
	return nil
}

func provision(t *testing.T, body map[string]any) (*httptest.ResponseRecorder, *memJobs) {
	t.Helper()
	jobs := &memJobs{}
	ops := operationapp.NewService(jobs, nil, nil, nil, nil, nil)
	repo := &memInstances{items: map[uuid.UUID]*instancedom.Instance{}}
	h := NewInstanceHandler(instanceapp.NewService(repo, nil, memSecrets{}, ops), nil)

	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/instances/provision", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	p := authapp.NewPrincipal(uuid.New(), "test@example.com", "instance:write")
	rr := httptest.NewRecorder()
	h.Provision(rr, req.WithContext(withPrincipal(req.Context(), p)))
	return rr, jobs
}

func provisionParams(t *testing.T, jobs *memJobs) operationapp.Params {
	t.Helper()
	if len(jobs.created) != 1 {
		t.Fatalf("created %d jobs, want 1", len(jobs.created))
	}
	var p operationapp.Params
	if err := json.Unmarshal(jobs.created[0].Params, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProvision_PGVector(t *testing.T) {
	rr, jobs := provision(t, map[string]any{
		"server_id": uuid.NewString(), "name": "vectors", "engine": "postgres",
		"engine_version": "17", "port": 5433, "pgvector": true,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
	p := provisionParams(t, jobs)
	if p.Image != "pgvector/pgvector" || p.Tag != "pg17" || p.Version != "17" {
		t.Errorf("params = image %q tag %q version %q, want pgvector/pgvector pg17 17", p.Image, p.Tag, p.Version)
	}
}

func TestProvision_DefaultImageUnchanged(t *testing.T) {
	rr, jobs := provision(t, map[string]any{
		"server_id": uuid.NewString(), "name": "plain", "engine": "postgres",
		"engine_version": "17", "port": 5433,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
	p := provisionParams(t, jobs)
	if p.Image != "postgres" || p.Tag != "" {
		t.Errorf("params = image %q tag %q, want postgres with no tag", p.Image, p.Tag)
	}
}

func TestProvision_PGVectorRejectedForMariaDB(t *testing.T) {
	rr, jobs := provision(t, map[string]any{
		"server_id": uuid.NewString(), "name": "maria", "engine": "mariadb",
		"engine_version": "11.4", "port": 3307, "pgvector": true,
	})
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want a validation error; body = %s", rr.Code, rr.Body)
	}
	if len(jobs.created) != 0 {
		t.Error("no job may be created for a rejected request")
	}
}
