package authzapp

import (
	"context"
	"testing"

	"github.com/google/uuid"

	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

type lineage struct {
	instanceServer map[uuid.UUID]uuid.UUID
	dbInstance     map[uuid.UUID]uuid.UUID
	backupDB       map[uuid.UUID]uuid.UUID
	jobs           map[uuid.UUID]struct {
		t  string
		id uuid.UUID
	}
}

func (l lineage) ServerOfInstance(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
	s, ok := l.instanceServer[id]
	if !ok {
		return uuid.Nil, apperr.NotFound("instance not found")
	}
	return s, nil
}
func (l lineage) LineageOfDatabase(ctx context.Context, id uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	inst, ok := l.dbInstance[id]
	if !ok {
		return uuid.Nil, uuid.Nil, apperr.NotFound("database not found")
	}
	srv, err := l.ServerOfInstance(ctx, inst)
	return inst, srv, err
}
func (l lineage) DatabaseOfBackup(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
	return l.backupDB[id], nil
}
func (l lineage) JobResource(_ context.Context, id uuid.UUID) (string, uuid.UUID, error) {
	j := l.jobs[id]
	return j.t, j.id, nil
}

// A fleet of two servers; a user granted one database on the first, and an
// operator granted the second server, may each see only their part.
func TestAncestryDrivesScopedAccess(t *testing.T) {
	srvA, srvB := uuid.New(), uuid.New()
	instA, instB := uuid.New(), uuid.New()
	dbA1, dbA2, dbB := uuid.New(), uuid.New(), uuid.New()
	backupA1, backupB := uuid.New(), uuid.New()
	jobA1, jobGlobal := uuid.New(), uuid.New()
	l := lineage{
		instanceServer: map[uuid.UUID]uuid.UUID{instA: srvA, instB: srvB},
		dbInstance:     map[uuid.UUID]uuid.UUID{dbA1: instA, dbA2: instA, dbB: instB},
		backupDB:       map[uuid.UUID]uuid.UUID{backupA1: dbA1, backupB: dbB},
		jobs: map[uuid.UUID]struct {
			t  string
			id uuid.UUID
		}{jobA1: {"database", dbA1}, jobGlobal: {"", uuid.Nil}},
	}
	rv := NewResolver(l)
	ctx := context.Background()
	anc := func(rt authz.ResourceType, id uuid.UUID) authz.Ancestry {
		t.Helper()
		a, err := rv.Ancestry(ctx, rt, id)
		if err != nil {
			t.Fatalf("%s %s: %v", rt, id, err)
		}
		return a
	}

	dbUser := []authz.Grant{{Permission: "database:read", Scope: authz.Scope{Type: authz.ScopeDatabase, ID: dbA1}}}
	operator := []authz.Grant{{Permission: "database:read", Scope: authz.Scope{Type: authz.ScopeServer, ID: srvB}}}
	admin := []authz.Grant{{Permission: "database:read", Scope: authz.Scope{Type: authz.ScopeGlobal}}}

	cases := []struct {
		who    string
		grants []authz.Grant
		rt     authz.ResourceType
		id     uuid.UUID
		want   bool
	}{
		{"db user, own database", dbUser, authz.ResourceDatabase, dbA1, true},
		{"db user, sibling database", dbUser, authz.ResourceDatabase, dbA2, false},
		{"db user, own database's backup", dbUser, authz.ResourceBackup, backupA1, true},
		{"db user, other backup", dbUser, authz.ResourceBackup, backupB, false},
		{"db user, own database's job", dbUser, authz.ResourceOperation, jobA1, true},
		{"db user, unscoped job", dbUser, authz.ResourceOperation, jobGlobal, false},
		{"db user, the server itself", dbUser, authz.ResourceServer, srvA, false},
		{"operator, database on their server", operator, authz.ResourceDatabase, dbB, true},
		{"operator, instance on their server", operator, authz.ResourceInstance, instB, true},
		{"operator, other server's database", operator, authz.ResourceDatabase, dbA1, false},
		{"admin, unscoped job", admin, authz.ResourceOperation, jobGlobal, true},
		{"admin, anything", admin, authz.ResourceDatabase, dbA2, true},
	}
	for _, c := range cases {
		if got := authz.Allow(c.grants, "database:read", anc(c.rt, c.id)); got != c.want {
			t.Errorf("%s: allow = %v, want %v", c.who, got, c.want)
		}
	}

	if _, err := rv.Ancestry(ctx, authz.ResourceDatabase, uuid.New()); apperr.KindOf(err) != apperr.KindNotFound {
		t.Errorf("unknown database: %v", err)
	}
}
