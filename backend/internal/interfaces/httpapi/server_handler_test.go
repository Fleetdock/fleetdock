package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	authapp "github.com/TajBrains/fleetdock/backend/internal/app/auth"
	serverapp "github.com/TajBrains/fleetdock/backend/internal/app/server"
	serverdom "github.com/TajBrains/fleetdock/backend/internal/domain/server"
	"github.com/TajBrains/fleetdock/backend/internal/platform/apperr"
)

// fakeService lets us drive the handler without the application/DB layers.
type fakeService struct {
	registerFn func(context.Context, serverapp.RegisterInput) (*serverdom.Server, error)
	getFn      func(context.Context, string) (*serverdom.Server, error)
	listFn     func(context.Context, serverapp.ListParams) (serverapp.ListResult, error)
}

func (f fakeService) Register(ctx context.Context, in serverapp.RegisterInput) (*serverdom.Server, error) {
	return f.registerFn(ctx, in)
}
func (f fakeService) Get(ctx context.Context, id string) (*serverdom.Server, error) {
	return f.getFn(ctx, id)
}
func (f fakeService) List(ctx context.Context, p serverapp.ListParams) (serverapp.ListResult, error) {
	return f.listFn(ctx, p)
}

// newTestServer builds a minimal server-routes mux and injects an
// all-permissions principal so handler tests bypass real authentication.
func newTestServer(svc ServersService) http.Handler {
	h := NewServerHandler(svc)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/servers", requirePerm("server:write", h.Register))
	mux.HandleFunc("GET /v1/servers", requirePerm("server:read", h.List))
	mux.HandleFunc("GET /v1/servers/{id}", requirePerm("server:read", h.Get))

	p := authapp.NewPrincipal(uuid.New(), "test@example.com", "server:read", "server:write")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), p)))
	})
}

func TestRegister_Created(t *testing.T) {
	svc := fakeService{registerFn: func(_ context.Context, in serverapp.RegisterInput) (*serverdom.Server, error) {
		return &serverdom.Server{
			ID: uuid.New(), Name: in.Name, Hostname: in.Hostname,
			Status: serverdom.StatusPending, Labels: map[string]string{}, Tags: []string{},
		}, nil
	}}
	body, _ := json.Marshal(map[string]any{"name": "db-1", "hostname": "db1.internal"})
	req := httptest.NewRequest(http.MethodPost, "/v1/servers", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	newTestServer(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rr.Code, rr.Body.String())
	}
	var got serverResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "db-1" || got.Status != "pending" {
		t.Errorf("unexpected response: %+v", got)
	}
}

func TestRegister_ValidationError(t *testing.T) {
	svc := fakeService{registerFn: func(_ context.Context, _ serverapp.RegisterInput) (*serverdom.Server, error) {
		return nil, apperr.Invalid("name", "name is required")
	}}
	body, _ := json.Marshal(map[string]any{"hostname": "h"})
	req := httptest.NewRequest(http.MethodPost, "/v1/servers", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	newTestServer(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rr.Code)
	}
}

func TestRegister_BadJSON(t *testing.T) {
	svc := fakeService{}
	req := httptest.NewRequest(http.MethodPost, "/v1/servers", bytes.NewReader([]byte("{not json")))
	rr := httptest.NewRecorder()

	newTestServer(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for bad JSON, got %d", rr.Code)
	}
}

func TestGet_NotFound(t *testing.T) {
	svc := fakeService{getFn: func(_ context.Context, _ string) (*serverdom.Server, error) {
		return nil, apperr.NotFound("server not found")
	}}
	req := httptest.NewRequest(http.MethodGet, "/v1/servers/"+uuid.NewString(), nil)
	rr := httptest.NewRecorder()

	newTestServer(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestList_OK(t *testing.T) {
	svc := fakeService{listFn: func(_ context.Context, _ serverapp.ListParams) (serverapp.ListResult, error) {
		return serverapp.ListResult{
			Items:  []*serverdom.Server{{ID: uuid.New(), Name: "db-1", Status: serverdom.StatusOnline, Labels: map[string]string{}, Tags: []string{}}},
			Total:  1,
			Limit:  20,
			Offset: 0,
		}, nil
	}}
	req := httptest.NewRequest(http.MethodGet, "/v1/servers?limit=20", nil)
	rr := httptest.NewRecorder()

	newTestServer(svc).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var got listServersResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Pagination.Total != 1 || len(got.Items) != 1 {
		t.Errorf("unexpected list response: %+v", got)
	}
}
