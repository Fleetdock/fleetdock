package instanceapp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/netsafe"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/sshtunnel"
)

func testKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block))
}

func fieldOf(err error) string {
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		return ""
	}
	return ae.Field
}

// tunnelFixture is newFixture with an SSH tunnel (password auth) in place
// and its host key already pinned.
func tunnelFixture(t *testing.T) (*Service, *fakeRepo, *fakeSecrets, *instancedom.Instance) {
	t.Helper()
	svc, repo, secrets, inst := newFixture(t)
	ref := sshSecretRef(inst.ID)
	raw, _ := sshtunnel.Credentials{Password: "tunnel-pw"}.Marshal()
	secrets.store[ref] = raw
	repo.items[inst.ID].SSH = &instancedom.SSHTunnel{
		Host: "bastion.example.com", Port: 22, User: "jump", Auth: instancedom.SSHAuthPassword,
		SecretRef: &ref, HostKey: "ssh-ed25519 AAAA pinned",
	}
	return svc, repo, secrets, inst
}

func TestRegister_ExternalWithSSHKey(t *testing.T) {
	repo := &fakeRepo{items: map[uuid.UUID]*instancedom.Instance{}}
	secrets := &fakeSecrets{store: map[string][]byte{}}
	svc := NewService(repo, nil, secrets, nil)
	key := testKey(t)

	inst, err := svc.Register(context.Background(), RegisterInput{
		Kind: "external", Name: "behind-bastion", Engine: "postgres", EngineVersion: "16",
		Host: "127.0.0.1", Port: 5432, Username: "postgres", Password: "pw",
		SSHTunnel: &SSHTunnelInput{Host: "bastion.example.com", Username: "jump", AuthMethod: "key", PrivateKey: key},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if inst.SSH == nil || inst.SSH.Port != 22 || inst.SSH.User != "jump" || inst.SSH.Auth != instancedom.SSHAuthKey {
		t.Fatalf("tunnel = %+v", inst.SSH)
	}
	ref := sshSecretRef(inst.ID)
	if inst.SSH.SecretRef == nil || *inst.SSH.SecretRef != ref {
		t.Fatalf("secret ref = %v", inst.SSH.SecretRef)
	}
	creds, err := sshtunnel.UnmarshalCredentials(secrets.store[ref])
	if err != nil || creds.PrivateKey != key {
		t.Fatalf("stored credentials = %+v, %v", creds, err)
	}
}

func TestRegister_SSHSkipsDBHostPolicyButChecksBastion(t *testing.T) {
	netsafe.ConfigureDB(&netsafe.DBPolicy{})
	defer netsafe.ConfigureDB(nil)
	repo := &fakeRepo{items: map[uuid.UUID]*instancedom.Instance{}}
	svc := NewService(repo, nil, &fakeSecrets{store: map[string][]byte{}}, nil)

	in := RegisterInput{
		Kind: "external", Name: "x", Engine: "postgres", EngineVersion: "16", Host: "127.0.0.1", Port: 5432,
		SSHTunnel: &SSHTunnelInput{Host: "10.0.0.5", Username: "jump", AuthMethod: "password", Password: "pw"},
	}
	if _, err := svc.Register(context.Background(), in); err != nil {
		t.Fatalf("loopback DB host behind a bastion should be allowed: %v", err)
	}

	in.Name = "y"
	in.SSHTunnel.Host = "169.254.169.254"
	_, err := svc.Register(context.Background(), in)
	if err == nil || fieldOf(err) != "ssh_tunnel.host" {
		t.Fatalf("metadata bastion must be refused on ssh_tunnel.host, got %v", err)
	}
}

func TestRegister_SSHValidation(t *testing.T) {
	cases := []struct {
		name  string
		kind  string
		in    SSHTunnelInput
		field string
	}{
		{"managed", "managed", SSHTunnelInput{Host: "b", Username: "u", AuthMethod: "password", Password: "p"}, "ssh_tunnel"},
		{"no host", "external", SSHTunnelInput{Username: "u", AuthMethod: "password", Password: "p"}, "ssh_tunnel.host"},
		{"no user", "external", SSHTunnelInput{Host: "b", AuthMethod: "password", Password: "p"}, "ssh_tunnel.username"},
		{"bad port", "external", SSHTunnelInput{Host: "b", Port: 70000, Username: "u", AuthMethod: "password", Password: "p"}, "ssh_tunnel.port"},
		{"bad method", "external", SSHTunnelInput{Host: "b", Username: "u", AuthMethod: "gssapi"}, "ssh_tunnel.auth_method"},
		{"no password", "external", SSHTunnelInput{Host: "b", Username: "u", AuthMethod: "password"}, "ssh_tunnel.password"},
		{"bad key", "external", SSHTunnelInput{Host: "b", Username: "u", AuthMethod: "key", PrivateKey: "nope"}, "ssh_tunnel.private_key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(&fakeRepo{items: map[uuid.UUID]*instancedom.Instance{}}, nil, &fakeSecrets{store: map[string][]byte{}}, nil)
			in := tc.in
			_, err := svc.Register(context.Background(), RegisterInput{
				Kind: tc.kind, ServerID: "6f1c1f3e-6d36-4f0e-9a43-3c1c3c7d7d10", Name: "x", Engine: "postgres",
				EngineVersion: "16", Host: "db", Port: 5432, SSHTunnel: &in,
			})
			if fieldOf(err) != tc.field {
				t.Fatalf("err = %v (field %q), want field %q", err, fieldOf(err), tc.field)
			}
		})
	}
}

func TestUpdate_AddTunnelRequiresAdminPassword(t *testing.T) {
	svc, repo, _, inst := newFixture(t)
	in := UpdateInput{SSHTunnel: &SSHTunnelInput{Host: "bastion", Username: "u", AuthMethod: "password", Password: "p"}}

	_, err := svc.Update(context.Background(), inst.ID.String(), in)
	if fieldOf(err) != "password" {
		t.Fatalf("adding a tunnel must require the admin password, got %v", err)
	}
	if repo.items[inst.ID].SSH != nil {
		t.Fatal("tunnel must not be saved")
	}

	in.Password = ptr("s3cret")
	got, err := svc.Update(context.Background(), inst.ID.String(), in)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.SSH == nil || got.SSH.Host != "bastion" || got.SSH.SecretRef == nil {
		t.Fatalf("tunnel = %+v", got.SSH)
	}
}

func TestUpdate_TunnelKeepsSecretAndPinOnSameBastion(t *testing.T) {
	svc, _, secrets, inst := tunnelFixture(t)
	before := string(secrets.store[sshSecretRef(inst.ID)])

	got, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{SSHTunnel: &SSHTunnelInput{
		Host: "bastion.example.com", Username: "jump", AuthMethod: "password",
	}})
	if err != nil {
		t.Fatalf("re-saving the same tunnel without its secret should work: %v", err)
	}
	if got.SSH.HostKey == "" {
		t.Error("pin must be kept on the same bastion")
	}
	if string(secrets.store[sshSecretRef(inst.ID)]) != before {
		t.Error("stored SSH secret must be untouched")
	}
}

func TestUpdate_NewBastionNeedsSecretsAndClearsPin(t *testing.T) {
	svc, _, _, inst := tunnelFixture(t)
	tunnel := &SSHTunnelInput{Host: "other-bastion", Username: "jump", AuthMethod: "password"}

	_, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{SSHTunnel: tunnel, Password: ptr("s3cret")})
	if fieldOf(err) != "ssh_tunnel.password" {
		t.Fatalf("moving the tunnel must require the SSH secret, got %v", err)
	}

	tunnel.Password = "new-pw"
	_, err = svc.Update(context.Background(), inst.ID.String(), UpdateInput{SSHTunnel: tunnel})
	if fieldOf(err) != "password" {
		t.Fatalf("moving the tunnel must require the admin password, got %v", err)
	}

	got, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{SSHTunnel: tunnel, Password: ptr("s3cret")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.SSH.HostKey != "" {
		t.Error("pin must be cleared for a new bastion")
	}
}

func TestUpdate_RotateSSHSecretAfterRowUpdate(t *testing.T) {
	svc, repo, secrets, inst := tunnelFixture(t)
	ref := sshSecretRef(inst.ID)
	repo.updateErr = apperr.Conflict("boom")
	in := UpdateInput{SSHTunnel: &SSHTunnelInput{Host: "bastion.example.com", Username: "jump", AuthMethod: "password", Password: "rotated"}}

	if _, err := svc.Update(context.Background(), inst.ID.String(), in); err == nil {
		t.Fatal("expected the update error")
	}
	if c, _ := sshtunnel.UnmarshalCredentials(secrets.store[ref]); c.Password != "tunnel-pw" {
		t.Fatal("rotation must not overwrite the secret when the row update fails")
	}

	repo.updateErr = nil
	if _, err := svc.Update(context.Background(), inst.ID.String(), in); err != nil {
		t.Fatalf("update: %v", err)
	}
	if c, _ := sshtunnel.UnmarshalCredentials(secrets.store[ref]); c.Password != "rotated" {
		t.Fatal("SSH secret should be rotated")
	}
}

func TestUpdate_RemoveTunnel(t *testing.T) {
	svc, _, secrets, inst := tunnelFixture(t)

	got, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{RemoveSSHTunnel: true, Password: ptr("s3cret")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.SSH != nil {
		t.Fatal("tunnel should be removed")
	}
	if _, ok := secrets.store[sshSecretRef(inst.ID)]; ok {
		t.Fatal("SSH secret should be deleted")
	}
}

func TestUpdate_RemoveTunnelChecksDBHostDirectly(t *testing.T) {
	netsafe.ConfigureDB(&netsafe.DBPolicy{})
	defer netsafe.ConfigureDB(nil)
	svc, repo, _, inst := tunnelFixture(t)
	repo.items[inst.ID].Host = ptr("127.0.0.1")

	_, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{RemoveSSHTunnel: true, Password: ptr("s3cret")})
	if fieldOf(err) != "host" {
		t.Fatalf("a loopback host must be refused once it is dialed directly, got %v", err)
	}
}

func TestUpdate_ResetHostKey(t *testing.T) {
	svc, _, _, inst := tunnelFixture(t)

	got, err := svc.Update(context.Background(), inst.ID.String(), UpdateInput{ResetSSHHostKey: true})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.SSH == nil || got.SSH.HostKey != "" {
		t.Fatalf("pin should be cleared, tunnel = %+v", got.SSH)
	}

	plain, _, _, plainInst := newFixture(t)
	if _, err := plain.Update(context.Background(), plainInst.ID.String(), UpdateInput{ResetSSHHostKey: true}); fieldOf(err) != "reset_ssh_host_key" {
		t.Fatalf("reset without a tunnel should be refused, got %v", err)
	}
}
