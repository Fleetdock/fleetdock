//go:build integration

package postgres

import (
	"context"
	"testing"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
)

func TestIntegrationInstanceDataAccess(t *testing.T) {
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

	inst, err := instancedom.NewExternal("data", instancedom.EnginePostgres, "16", "127.0.0.1", 5432, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, inst); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.GetByID(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DataAccess != instancedom.DataAccessAdmin || got.DataUsername != nil {
		t.Fatalf("new instance data access = %q user=%v, want admin", got.DataAccess, got.DataUsername)
	}

	// A login without a username is refused by the schema.
	login := instancedom.DataAccessLogin
	if err := repo.Update(ctx, inst.ID, instancedom.UpdateFields{DataAccess: &login}); err == nil {
		t.Fatal("login mode without a username must be rejected")
	}

	ref := "instance/" + inst.ID.String() + "/data"
	if err := secrets.Upsert(ctx, &secretdom.Secret{Ref: ref, Kind: secretdom.KindPostgresUser,
		Ciphertext: []byte("c"), EncryptedDataKey: []byte("k"), KeyID: "v1", Nonce: []byte("n")}); err != nil {
		t.Fatalf("secret: %v", err)
	}
	user := "reader"
	if err := repo.Update(ctx, inst.ID, instancedom.UpdateFields{
		DataAccess: &login, DataLogin: &instancedom.Credentials{Username: &user, RootSecretRef: &ref},
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err = repo.GetByID(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DataAccess != login || got.DataUsername == nil || *got.DataUsername != user ||
		got.DataSecretRef == nil || *got.DataSecretRef != ref {
		t.Fatalf("after update: %q user=%v ref=%v", got.DataAccess, got.DataUsername, got.DataSecretRef)
	}

	page, err := repo.List(ctx, instancedom.ListFilter{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].DataAccess != login {
		t.Fatalf("list: %+v, %v", page, err)
	}
}
