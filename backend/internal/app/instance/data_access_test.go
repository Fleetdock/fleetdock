package instanceapp

import (
	"context"
	"testing"

	"github.com/google/uuid"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
)

func TestRegister_DefaultsToAdminLogin(t *testing.T) {
	repo := &fakeRepo{items: map[uuid.UUID]*instancedom.Instance{}}
	svc := NewService(repo, nil, &fakeSecrets{store: map[string][]byte{}}, nil)
	inst, err := svc.Register(context.Background(), RegisterInput{
		Kind: "external", Name: "db", Engine: "postgres", EngineVersion: "16", Host: "db.example.com", Port: 5432,
		Username: "postgres", Password: "pw",
	})
	if err != nil {
		t.Fatal(err)
	}
	if inst.DataAccessOrDefault() != instancedom.DataAccessAdmin || inst.DataUsername != nil {
		t.Errorf("data access = %q (%v), want admin with no data login", inst.DataAccess, inst.DataUsername)
	}
}

func TestRegister_WithDataLogin(t *testing.T) {
	repo := &fakeRepo{items: map[uuid.UUID]*instancedom.Instance{}}
	secrets := &fakeSecrets{store: map[string][]byte{}}
	svc := NewService(repo, nil, secrets, nil)
	inst, err := svc.Register(context.Background(), RegisterInput{
		Kind: "external", Name: "db", Engine: "postgres", EngineVersion: "16", Host: "db.example.com", Port: 5432,
		Username: "postgres", Password: "pw",
		DataAccess: DataAccessInput{Mode: ptr("login"), Username: ptr("reader"), Password: ptr("reader-pw")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if inst.DataAccess != instancedom.DataAccessLogin || inst.DataUsername == nil || *inst.DataUsername != "reader" {
		t.Fatalf("data access = %q user %v", inst.DataAccess, inst.DataUsername)
	}
	if got := string(secrets.store[dataSecretRef(inst.ID)]); got != "reader-pw" {
		t.Errorf("stored data password = %q", got)
	}
}

func TestUpdate_DataLoginValidation(t *testing.T) {
	cases := []struct {
		name  string
		in    DataAccessInput
		field string
	}{
		{"bad mode", DataAccessInput{Mode: ptr("root")}, "data_access"},
		{"login without username", DataAccessInput{Mode: ptr("login"), Password: ptr("pw")}, "data_username"},
		{"login without password", DataAccessInput{Mode: ptr("login"), Username: ptr("reader")}, "data_password"},
		{"login fields in admin mode", DataAccessInput{Mode: ptr("admin"), Username: ptr("reader")}, "data_username"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, _, _, inst := newFixture(t)
			_, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{DataAccess: c.in})
			if fieldOf(err) != c.field {
				t.Fatalf("err = %v, want field %q", err, c.field)
			}
		})
	}
}

// dataLoginFixture is newFixture with a dedicated data login configured.
func dataLoginFixture(t *testing.T) (*Service, *fakeRepo, *fakeSecrets, *instancedom.Instance) {
	t.Helper()
	svc, repo, secrets, inst := newFixture(t)
	ref := dataSecretRef(inst.ID)
	secrets.store[ref] = []byte("reader-pw")
	stored := repo.items[inst.ID]
	stored.DataAccess, stored.DataUsername, stored.DataSecretRef = instancedom.DataAccessLogin, ptr("reader"), &ref
	return svc, repo, secrets, inst
}

func TestUpdate_SwitchingAwayFromLoginDeletesIt(t *testing.T) {
	svc, _, secrets, inst := dataLoginFixture(t)
	out, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{DataAccess: DataAccessInput{Mode: ptr("managed")}})
	if err != nil {
		t.Fatal(err)
	}
	if out.DataAccess != instancedom.DataAccessManaged || out.DataUsername != nil || out.DataSecretRef != nil {
		t.Errorf("after switch: %q user=%v ref=%v", out.DataAccess, out.DataUsername, out.DataSecretRef)
	}
	if _, ok := secrets.store[dataSecretRef(inst.ID)]; ok {
		t.Error("data login secret should be deleted")
	}
}

func TestUpdate_DataLoginRotatesPasswordInPlace(t *testing.T) {
	svc, _, secrets, inst := dataLoginFixture(t)
	if _, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{DataAccess: DataAccessInput{Password: ptr("new-pw")}}); err != nil {
		t.Fatal(err)
	}
	if got := string(secrets.store[dataSecretRef(inst.ID)]); got != "new-pw" {
		t.Errorf("data password = %q, want rotated", got)
	}
}

func TestUpdate_DataLoginRenameNeedsPassword(t *testing.T) {
	svc, _, _, inst := dataLoginFixture(t)
	_, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{DataAccess: DataAccessInput{Username: ptr("other")}})
	if fieldOf(err) != "data_password" {
		t.Fatalf("err = %v, want data_password", err)
	}
}

func TestUpdate_HostChangeNeedsDataPasswordToo(t *testing.T) {
	svc, _, _, inst := dataLoginFixture(t)
	_, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{
		Host: ptr("203.0.113.10"), Password: ptr("s3cret"),
	})
	if fieldOf(err) != "data_password" {
		t.Fatalf("err = %v, want data_password: the data login must not follow a host change unchallenged", err)
	}
}
