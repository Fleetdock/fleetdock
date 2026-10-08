package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	authapp "github.com/Fleetdock/fleetdock/backend/internal/app/auth"
	instanceapp "github.com/Fleetdock/fleetdock/backend/internal/app/instance"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// memInstances is an in-memory instance repository for handler tests.
type memInstances struct {
	items map[uuid.UUID]*instancedom.Instance
}

func (r *memInstances) Create(_ context.Context, in *instancedom.Instance) error {
	r.items[in.ID] = in
	return nil
}
func (r *memInstances) GetByID(_ context.Context, id uuid.UUID) (*instancedom.Instance, error) {
	in, ok := r.items[id]
	if !ok {
		return nil, apperr.NotFound("instance not found")
	}
	c := *in
	return &c, nil
}
func (r *memInstances) List(context.Context, instancedom.ListFilter) (instancedom.Page, error) {
	return instancedom.Page{}, nil
}
func (r *memInstances) Update(_ context.Context, id uuid.UUID, f instancedom.UpdateFields) error {
	in := r.items[id]
	if f.Name != nil {
		in.Name = *f.Name
	}
	if f.Host != nil {
		in.Host = f.Host
	}
	if f.Port != nil {
		in.Port = *f.Port
	}
	if f.Credentials != nil {
		in.Username, in.RootSecretRef = f.Credentials.Username, f.Credentials.RootSecretRef
	}
	switch {
	case f.RemoveSSH:
		in.SSH = nil
	case f.SSH != nil:
		t := *f.SSH
		in.SSH = &t
	}
	return nil
}
func (r *memInstances) SetRootSecretRef(context.Context, uuid.UUID, string) error { return nil }
func (r *memInstances) SetStatus(context.Context, uuid.UUID, instancedom.Status) error {
	return nil
}
func (r *memInstances) SetContainerID(context.Context, uuid.UUID, string) error { return nil }
func (r *memInstances) SoftDelete(context.Context, uuid.UUID) error             { return nil }

type memSecrets map[string][]byte

func (s memSecrets) Put(_ context.Context, ref string, _ secretdom.Kind, p []byte) error {
	s[ref] = p
	return nil
}
func (s memSecrets) Get(_ context.Context, ref string) ([]byte, error) { return s[ref], nil }
func (s memSecrets) Delete(_ context.Context, ref string) error {
	delete(s, ref)
	return nil
}

func newInstanceTestServer(t *testing.T) (http.Handler, *instancedom.Instance, memSecrets) {
	t.Helper()
	user := "root"
	inst, err := instancedom.NewExternal("primary", instancedom.EngineMariaDB, "11.4", "db.example.com", 3306, &user, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := "instance/" + inst.ID.String() + "/root"
	inst.RootSecretRef = &ref
	secrets := memSecrets{ref: []byte("s3cret")}
	repo := &memInstances{items: map[uuid.UUID]*instancedom.Instance{inst.ID: inst}}

	h := NewInstanceHandler(instanceapp.NewService(repo, nil, secrets, nil), nil)
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /v1/instances/{id}", requirePerm("instance:write", h.Update))
	p := authapp.NewPrincipal(uuid.New(), "test@example.com", "instance:write")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), p)))
	}), inst, secrets
}

func patchInstance(t *testing.T, h http.Handler, id uuid.UUID, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/v1/instances/"+id.String(), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestUpdateInstance_Rename(t *testing.T) {
	h, inst, _ := newInstanceTestServer(t)

	rr := patchInstance(t, h, inst.ID, map[string]any{"name": "renamed"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["name"] != "renamed" {
		t.Errorf("name = %v", got["name"])
	}
	if strings.Contains(rr.Body.String(), "s3cret") {
		t.Error("response must never include the password")
	}
}

func TestUpdateInstance_HostChangeWithoutPasswordIs400(t *testing.T) {
	h, inst, secrets := newInstanceTestServer(t)

	rr := patchInstance(t, h, inst.ID, map[string]any{"host": "attacker.example.com"})
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 4xx validation error; body = %s", rr.Code, rr.Body)
	}
	if string(secrets["instance/"+inst.ID.String()+"/root"]) != "s3cret" {
		t.Error("stored password must be untouched")
	}
}

func TestUpdateInstance_ClearCredentials(t *testing.T) {
	h, inst, secrets := newInstanceTestServer(t)

	rr := patchInstance(t, h, inst.ID, map[string]any{"username": ""})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["has_credentials"] != false {
		t.Errorf("has_credentials = %v, want false", got["has_credentials"])
	}
	if len(secrets) != 0 {
		t.Error("clearing credentials should delete the stored password")
	}
}

func (r *memInstances) SetHealth(context.Context, uuid.UUID, instancedom.Health) error { return nil }
func (r *memInstances) PinSSHHostKey(context.Context, uuid.UUID, string) error         { return nil }

func TestUpdateInstance_SSHTunnelNeverEchoesSecrets(t *testing.T) {
	h, inst, secrets := newInstanceTestServer(t)

	rr := patchInstance(t, h, inst.ID, map[string]any{
		"password": "s3cret",
		"ssh_tunnel": map[string]any{
			"host": "bastion.example.com", "username": "jump", "auth_method": "password", "password": "tunnel-secret",
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
	if strings.Contains(rr.Body.String(), "tunnel-secret") || strings.Contains(rr.Body.String(), "s3cret") {
		t.Fatal("response must never include SSH or admin secrets")
	}
	var got struct {
		SSHTunnel *struct {
			Host               string  `json:"host"`
			Port               int     `json:"port"`
			Username           string  `json:"username"`
			AuthMethod         string  `json:"auth_method"`
			HostKeyFingerprint *string `json:"host_key_fingerprint"`
		} `json:"ssh_tunnel"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.SSHTunnel == nil || got.SSHTunnel.Host != "bastion.example.com" || got.SSHTunnel.Port != 22 ||
		got.SSHTunnel.AuthMethod != "password" || got.SSHTunnel.HostKeyFingerprint != nil {
		t.Fatalf("ssh_tunnel = %+v", got.SSHTunnel)
	}
	if _, ok := secrets["instance/"+inst.ID.String()+"/ssh"]; !ok {
		t.Fatal("SSH secret should be stored")
	}

	rr = patchInstance(t, h, inst.ID, map[string]any{"remove_ssh_tunnel": true, "password": "s3cret"})
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ssh_tunnel":null`) {
		t.Fatalf("remove: status = %d, body = %s", rr.Code, rr.Body)
	}
}
