package instanceapp

import (
	"context"
	"testing"

	"github.com/google/uuid"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/netsafe"
)

// ---- fakes ----

var _ instancedom.Repository = (*fakeRepo)(nil)

type fakeRepo struct {
	items     map[uuid.UUID]*instancedom.Instance
	updateErr error
}

func (r *fakeRepo) Create(_ context.Context, in *instancedom.Instance) error {
	r.items[in.ID] = in
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*instancedom.Instance, error) {
	in, ok := r.items[id]
	if !ok {
		return nil, apperr.NotFound("instance not found")
	}
	clone := *in
	return &clone, nil
}

func (r *fakeRepo) List(_ context.Context, _ instancedom.ListFilter) (instancedom.Page, error) {
	return instancedom.Page{}, nil
}

func (r *fakeRepo) Update(_ context.Context, id uuid.UUID, f instancedom.UpdateFields) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	in, ok := r.items[id]
	if !ok {
		return apperr.NotFound("instance not found")
	}
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
		in.Username = f.Credentials.Username
		in.RootSecretRef = f.Credentials.RootSecretRef
	}
	switch {
	case f.RemoveSSH:
		in.SSH = nil
	case f.SSH != nil:
		t := *f.SSH
		in.SSH = &t
	case f.ResetSSHHostKey && in.SSH != nil:
		t := *in.SSH
		t.HostKey = ""
		in.SSH = &t
	}
	if f.DataAccess != nil {
		in.DataAccess = *f.DataAccess
	}
	if f.DataLogin != nil {
		in.DataUsername, in.DataSecretRef = f.DataLogin.Username, f.DataLogin.RootSecretRef
	}
	return nil
}

func (r *fakeRepo) SetRootSecretRef(_ context.Context, _ uuid.UUID, _ string) error { return nil }
func (r *fakeRepo) SetStatus(_ context.Context, _ uuid.UUID, _ instancedom.Status) error {
	return nil
}
func (r *fakeRepo) SetContainerID(_ context.Context, _ uuid.UUID, _ string) error { return nil }
func (r *fakeRepo) SoftDelete(_ context.Context, _ uuid.UUID) error               { return nil }

type fakeSecrets struct {
	store map[string][]byte
}

func (s *fakeSecrets) Put(_ context.Context, ref string, _ secretdom.Kind, plaintext []byte) error {
	s.store[ref] = plaintext
	return nil
}

func (s *fakeSecrets) Get(_ context.Context, ref string) ([]byte, error) {
	v, ok := s.store[ref]
	if !ok {
		return nil, apperr.NotFound("secret not found")
	}
	return v, nil
}

func (s *fakeSecrets) Delete(_ context.Context, ref string) error {
	delete(s.store, ref)
	return nil
}

// ---- helpers ----

func ptr[T any](v T) *T { return &v }

// newFixture builds a service holding one external instance with admin
// credentials already configured.
func newFixture(t *testing.T) (*Service, *fakeRepo, *fakeSecrets, *instancedom.Instance) {
	t.Helper()
	inst, err := instancedom.NewExternal("primary", instancedom.EngineMariaDB, "11.4", "db.example.com", 3306, ptr("root"), nil, nil)
	if err != nil {
		t.Fatalf("build instance: %v", err)
	}
	ref := rootSecretRef(inst.ID)
	inst.RootSecretRef = &ref

	repo := &fakeRepo{items: map[uuid.UUID]*instancedom.Instance{inst.ID: inst}}
	secrets := &fakeSecrets{store: map[string][]byte{ref: []byte("s3cret")}}
	return NewService(repo, nil, secrets, nil), repo, secrets, inst
}

// ---- tests ----

func TestUpdate_RenamesInstance(t *testing.T) {
	svc, _, _, inst := newFixture(t)

	got, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Name: ptr("  renamed  ")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Name != "renamed" {
		t.Errorf("name = %q, want %q (should be trimmed)", got.Name, "renamed")
	}
}

func TestUpdate_RejectsEmptyName(t *testing.T) {
	svc, _, _, inst := newFixture(t)

	if _, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Name: ptr("   ")}); err == nil {
		t.Fatal("expected an error for a blank name")
	}
}

func TestUpdate_RotatesPasswordInPlace(t *testing.T) {
	svc, _, secrets, inst := newFixture(t)
	ref := rootSecretRef(inst.ID)

	got, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Password: ptr("newpass")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !got.HasCredentials() {
		t.Error("instance should still have credentials after a rotation")
	}
	if string(secrets.store[ref]) != "newpass" {
		t.Errorf("stored secret = %q, want %q", secrets.store[ref], "newpass")
	}
	// The ref is deterministic, so rotation must overwrite rather than leaving
	// the superseded ciphertext behind under a second key.
	if len(secrets.store) != 1 {
		t.Errorf("secret store holds %d entries, want 1 (rotation should not orphan)", len(secrets.store))
	}
}

func TestUpdate_AddsCredentialsToBareInstance(t *testing.T) {
	svc, repo, secrets, inst := newFixture(t)
	// Strip the credentials to model an instance registered without them.
	repo.items[inst.ID].Username = nil
	repo.items[inst.ID].RootSecretRef = nil
	delete(secrets.store, rootSecretRef(inst.ID))

	got, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{
		Username: ptr("admin"),
		Password: ptr("hunter2"),
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !got.HasCredentials() {
		t.Fatal("instance should have credentials after adding them")
	}
	if *got.Username != "admin" {
		t.Errorf("username = %q, want %q", *got.Username, "admin")
	}
}

func TestUpdate_EmptyUsernameClearsCredentials(t *testing.T) {
	svc, _, secrets, inst := newFixture(t)

	got, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Username: ptr("")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.HasCredentials() {
		t.Error("clearing the username should leave the instance metadata-only")
	}
	if len(secrets.store) != 0 {
		t.Error("clearing the username should delete the stored password")
	}
}

func TestUpdate_EmptyPasswordKeepsUsername(t *testing.T) {
	svc, _, secrets, inst := newFixture(t)

	got, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Password: ptr("")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Username == nil || *got.Username != "root" {
		t.Error("removing the password should keep the admin username")
	}
	if got.HasCredentials() {
		t.Error("without a stored password the instance has no usable credentials")
	}
	if len(secrets.store) != 0 {
		t.Error("the stored password should have been deleted")
	}
}

func TestUpdate_PasswordWithoutUsernameIsRejected(t *testing.T) {
	svc, repo, secrets, inst := newFixture(t)
	repo.items[inst.ID].Username = nil
	repo.items[inst.ID].RootSecretRef = nil
	delete(secrets.store, rootSecretRef(inst.ID))

	_, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Password: ptr("hunter2")})
	if err == nil {
		t.Fatal("expected an error when setting a password with no username")
	}
	if len(secrets.store) != 0 {
		t.Error("a rejected update must not write to the secret store")
	}
}

func TestUpdate_HostRejectedOnManagedInstance(t *testing.T) {
	inst, err := instancedom.NewManaged(uuid.New(), "primary", instancedom.EngineMariaDB, "11.4", 3306, ptr("root"), nil, nil)
	if err != nil {
		t.Fatalf("build instance: %v", err)
	}
	repo := &fakeRepo{items: map[uuid.UUID]*instancedom.Instance{inst.ID: inst}}
	svc := NewService(repo, nil, &fakeSecrets{store: map[string][]byte{}}, nil)

	if _, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Host: ptr("db.example.com")}); err == nil {
		t.Fatal("expected an error setting host on a managed instance")
	}
}

func TestUpdate_PortRejectedOnProvisionedInstance(t *testing.T) {
	inst, err := instancedom.NewProvisioned(uuid.New(), "primary", instancedom.EngineMariaDB, "11.4", 3306)
	if err != nil {
		t.Fatalf("build instance: %v", err)
	}
	inst.ContainerID = ptr("abc123")
	repo := &fakeRepo{items: map[uuid.UUID]*instancedom.Instance{inst.ID: inst}}
	svc := NewService(repo, nil, &fakeSecrets{store: map[string][]byte{}}, nil)

	if _, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Port: ptr(5432)}); err == nil {
		t.Fatal("expected an error changing the port of a provisioned instance")
	}
}

func TestUpdate_RejectsOutOfRangePort(t *testing.T) {
	svc, _, _, inst := newFixture(t)

	if _, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Port: ptr(70000)}); err == nil {
		t.Fatal("expected an error for a port above 65535")
	}
}

func TestUpdate_NoFieldsIsANoop(t *testing.T) {
	svc, _, secrets, inst := newFixture(t)

	got, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Name != "primary" || !got.HasCredentials() {
		t.Error("an empty update should change nothing")
	}
	if string(secrets.store[rootSecretRef(inst.ID)]) != "s3cret" {
		t.Error("an empty update should not touch the secret store")
	}
}

func TestUpdate_HostChangeRequiresPassword(t *testing.T) {
	svc, repo, secrets, inst := newFixture(t)

	_, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Host: ptr("evil.example.com")})
	if err == nil {
		t.Fatal("expected changing the host without re-entering the password to fail")
	}
	if got := *repo.items[inst.ID].Host; got != "db.example.com" {
		t.Errorf("host = %q, want unchanged", got)
	}
	if string(secrets.store[rootSecretRef(inst.ID)]) != "s3cret" {
		t.Error("stored password must be untouched")
	}
}

func TestUpdate_HostChangeWithPasswordSucceeds(t *testing.T) {
	svc, _, secrets, inst := newFixture(t)

	got, err := svc.Update(context.Background(), inst.ID.String(),
		UpdateInput{Host: ptr("db2.example.com"), Password: ptr("n3w")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if *got.Host != "db2.example.com" {
		t.Errorf("host = %q", *got.Host)
	}
	if string(secrets.store[rootSecretRef(inst.ID)]) != "n3w" {
		t.Error("password should be rotated")
	}
}

func TestUpdate_SameHostNeedsNoPassword(t *testing.T) {
	svc, _, _, inst := newFixture(t)

	if _, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Host: ptr("db.example.com")}); err != nil {
		t.Fatalf("re-saving the same host should not require the password: %v", err)
	}
}

func TestUpdate_FailedRowUpdateKeepsSecrets(t *testing.T) {
	svc, repo, secrets, inst := newFixture(t)
	repo.updateErr = apperr.Conflict("name taken")
	ref := rootSecretRef(inst.ID)

	if _, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Password: ptr("")}); err == nil {
		t.Fatal("expected the update error")
	}
	if string(secrets.store[ref]) != "s3cret" {
		t.Error("clearing the password must not delete the secret when the row update fails")
	}

	if _, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{Password: ptr("other")}); err == nil {
		t.Fatal("expected the update error")
	}
	if string(secrets.store[ref]) != "s3cret" {
		t.Error("rotation must not overwrite the secret when the row update fails")
	}
}

func TestUpdate_RejectsDisallowedHost(t *testing.T) {
	netsafe.ConfigureDB(&netsafe.DBPolicy{})
	defer netsafe.ConfigureDB(nil)
	svc, _, _, inst := newFixture(t)

	_, err := svc.Update(context.Background(), inst.ID.String(),
		UpdateInput{Host: ptr("169.254.169.254"), Password: ptr("x")})
	if err == nil {
		t.Fatal("expected metadata address to be refused")
	}
}

func TestUpdate_RejectsBadTLSMode(t *testing.T) {
	svc, _, _, inst := newFixture(t)
	if _, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{TLSMode: ptr("maybe")}); err == nil {
		t.Fatal("expected invalid tls_mode to be refused")
	}
}

func (r *fakeRepo) SetHealth(context.Context, uuid.UUID, instancedom.Health) error { return nil }
func (r *fakeRepo) PinSSHHostKey(context.Context, uuid.UUID, string) error         { return nil }
