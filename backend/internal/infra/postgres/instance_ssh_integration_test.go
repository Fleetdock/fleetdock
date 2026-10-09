//go:build integration

package postgres

import (
	"context"
	"testing"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
)

func TestIntegrationInstanceSSHTunnel(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, scratchDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewInstanceRepository(pool)
	secrets := NewSecretRepository(pool)

	inst, err := instancedom.NewExternal("tunnelled", instancedom.EnginePostgres, "16", "127.0.0.1", 5432, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := "instance/" + inst.ID.String() + "/ssh"
	if err := secrets.Upsert(ctx, &secretdom.Secret{Ref: ref, Kind: secretdom.KindSSHKey,
		Ciphertext: []byte("c"), EncryptedDataKey: []byte("k"), KeyID: "v1", Nonce: []byte("n")}); err != nil {
		t.Fatalf("secret: %v", err)
	}
	inst.SSH = &instancedom.SSHTunnel{Host: "bastion", Port: 2222, User: "jump", Auth: instancedom.SSHAuthKey, SecretRef: &ref}
	if err := repo.Create(ctx, inst); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByID(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SSH == nil || got.SSH.Host != "bastion" || got.SSH.Port != 2222 || got.SSH.User != "jump" ||
		got.SSH.Auth != instancedom.SSHAuthKey || got.SSH.SecretRef == nil || *got.SSH.SecretRef != ref || got.SSH.HostKey != "" {
		t.Fatalf("tunnel = %+v", got.SSH)
	}

	// The first pin wins; a second one is ignored.
	if err := repo.PinSSHHostKey(ctx, inst.ID, "ssh-ed25519 FIRST"); err != nil {
		t.Fatal(err)
	}
	if err := repo.PinSSHHostKey(ctx, inst.ID, "ssh-ed25519 SECOND"); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetByID(ctx, inst.ID)
	if got.SSH.HostKey != "ssh-ed25519 FIRST" {
		t.Fatalf("host key = %q", got.SSH.HostKey)
	}
	if got.Version != inst.Version {
		t.Errorf("pinning must not bump the version (%d -> %d)", inst.Version, got.Version)
	}

	if err := repo.Update(ctx, inst.ID, instancedom.UpdateFields{ResetSSHHostKey: true}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetByID(ctx, inst.ID)
	if got.SSH == nil || got.SSH.HostKey != "" {
		t.Fatalf("reset: tunnel = %+v", got.SSH)
	}

	if err := repo.Update(ctx, inst.ID, instancedom.UpdateFields{RemoveSSH: true}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetByID(ctx, inst.ID)
	if got.SSH != nil {
		t.Fatalf("remove: tunnel = %+v", got.SSH)
	}

	// A tunnel on a managed instance is refused by the schema.
	_, err = pool.Exec(ctx, `UPDATE instances SET kind = 'managed', ssh_host = 'b', ssh_user = 'u', ssh_auth = 'key' WHERE id = $1`, inst.ID)
	if err == nil {
		t.Fatal("expected the instances_ssh_external / kind constraints to refuse an SSH tunnel on a managed instance")
	}
}
