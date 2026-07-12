package tokenapp

import (
	"context"
	"testing"

	"github.com/google/uuid"

	tokendom "github.com/Fleetdock/fleetdock/backend/internal/domain/token"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

type fakeTokenRepo struct {
	created *tokendom.Token
}

func (r *fakeTokenRepo) Create(_ context.Context, t *tokendom.Token, _ string) error {
	r.created = t
	return nil
}
func (r *fakeTokenRepo) ListByUser(_ context.Context, _ uuid.UUID) ([]*tokendom.Token, error) {
	return nil, nil
}
func (r *fakeTokenRepo) Revoke(_ context.Context, _, _ uuid.UUID) error { return nil }
func (r *fakeTokenRepo) ResolveHash(_ context.Context, _ string) (tokendom.Lookup, error) {
	return tokendom.Lookup{}, apperr.NotFound("not found")
}

func TestCreate_RejectsUnknownScope(t *testing.T) {
	svc := NewService(&fakeTokenRepo{})
	_, err := svc.Create(context.Background(), CreateInput{
		UserID: uuid.New(), Name: "t", Scopes: []string{"not:aperm"},
	})
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid, got %v", apperr.KindOf(err))
	}
}

func TestCreate_RejectsScopeUserLacks(t *testing.T) {
	svc := NewService(&fakeTokenRepo{})
	_, err := svc.Create(context.Background(), CreateInput{
		UserID: uuid.New(), Name: "t",
		Scopes:        []string{"database:write"},
		AllowedScopes: []string{"database:read"}, // user only holds read
	})
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid for scope the user lacks, got %v", apperr.KindOf(err))
	}
}

func TestCreate_AllowsHeldScopes(t *testing.T) {
	repo := &fakeTokenRepo{}
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), CreateInput{
		UserID: uuid.New(), Name: "ci",
		Scopes:        []string{"database:read"},
		AllowedScopes: []string{"database:read", "database:write"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.Plaintext == "" {
		t.Fatal("expected a plaintext token")
	}
	if repo.created == nil || len(repo.created.Scopes) != 1 {
		t.Fatalf("expected persisted token with one scope, got %+v", repo.created)
	}
}
