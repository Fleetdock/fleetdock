//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	tokendom "github.com/Fleetdock/fleetdock/backend/internal/domain/token"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

func TestIntegrationTokensAdministratorView(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, scratchDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	ann, bob := uuid.New(), uuid.New()
	for id, email := range map[uuid.UUID]string{ann: "ann@x.dev", bob: "bob@x.dev"} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, 'n', 'x')`, id, email); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewTokenRepository(pool)
	past := time.Now().Add(-time.Hour)
	mk := func(owner uuid.UUID, name string, expires *time.Time) *tokendom.Token {
		tok := &tokendom.Token{ID: uuid.New(), UserID: owner, Name: name, Prefix: "fleetd_" + name[:3], Scopes: []string{}, ExpiresAt: expires}
		if err := repo.Create(ctx, tok, uuid.NewString()); err != nil {
			t.Fatal(err)
		}
		return tok
	}
	annCI := mk(ann, "ann-ci", nil)
	mk(ann, "ann-old", &past) // expired: not listed
	bobCLI := mk(bob, "bob-cli", nil)

	list, err := repo.ListActive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[uuid.UUID]string{}
	for _, tok := range list {
		got[tok.ID] = tok.OwnerEmail
	}
	if len(got) != 2 || got[annCI.ID] != "ann@x.dev" || got[bobCLI.ID] != "bob@x.dev" {
		t.Fatalf("active tokens = %v", got)
	}

	owner, err := repo.RevokeAny(ctx, bobCLI.ID)
	if err != nil || owner != bob {
		t.Fatalf("RevokeAny = %s, %v", owner, err)
	}
	if _, err := repo.RevokeAny(ctx, bobCLI.ID); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("revoking twice: %v", err)
	}
	if list, _ := repo.ListActive(ctx); len(list) != 1 || list[0].ID != annCI.ID {
		t.Fatalf("after revoking: %+v", list)
	}
}
