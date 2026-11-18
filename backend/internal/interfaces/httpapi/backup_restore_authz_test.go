package httpapi

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	authapp "github.com/Fleetdock/fleetdock/backend/internal/app/auth"
	authzapp "github.com/Fleetdock/fleetdock/backend/internal/app/authz"
	backupapp "github.com/Fleetdock/fleetdock/backend/internal/app/backup"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// lineageRepo places instances and databases on servers for the resolver.
type lineageRepo struct {
	instServer map[uuid.UUID]uuid.UUID
	dbInst     map[uuid.UUID]uuid.UUID
}

func (r lineageRepo) ServerOfInstance(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
	return r.instServer[id], nil
}
func (r lineageRepo) LineageOfDatabase(_ context.Context, id uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	inst := r.dbInst[id]
	return inst, r.instServer[inst], nil
}
func (r lineageRepo) DatabaseOfBackup(context.Context, uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, nil
}
func (r lineageRepo) JobResource(context.Context, uuid.UUID) (string, uuid.UUID, error) {
	return "", uuid.Nil, nil
}

func TestAuthorizeRestoreTarget(t *testing.T) {
	serverA, serverB := uuid.New(), uuid.New()
	instA, instB := uuid.New(), uuid.New()
	ownDB, otherDBOnA, dbOnB := uuid.New(), uuid.New(), uuid.New()
	rv := authzapp.NewResolver(lineageRepo{
		instServer: map[uuid.UUID]uuid.UUID{instA: serverA, instB: serverB},
		dbInst:     map[uuid.UUID]uuid.UUID{ownDB: instA, otherDBOnA: instA, dbOnB: instB},
	})
	h := &BackupHandler{resolver: rv}

	serverOperator := newScopedPrincipal("database:write", authz.Scope{Type: authz.ScopeServer, ID: serverA})
	dbOperator := newScopedPrincipal("database:write", authz.Scope{Type: authz.ScopeDatabase, ID: ownDB})
	ptr := func(id uuid.UUID) *uuid.UUID { return &id }

	cases := []struct {
		name    string
		caller  *authapp.Principal
		target  backupapp.RestoreTarget
		allowed bool
	}{
		{"server operator, new database on own server", serverOperator,
			backupapp.RestoreTarget{InstanceID: instA, Database: "copy"}, true},
		{"server operator, existing database on own server", serverOperator,
			backupapp.RestoreTarget{InstanceID: instA, Database: "billing", Existing: ptr(otherDBOnA)}, true},
		{"server operator, new database on another server", serverOperator,
			backupapp.RestoreTarget{InstanceID: instB, Database: "copy"}, false},
		{"server operator, existing database on another server", serverOperator,
			backupapp.RestoreTarget{InstanceID: instB, Database: "billing", Existing: ptr(dbOnB)}, false},
		{"server operator, system database", serverOperator,
			backupapp.RestoreTarget{InstanceID: instA, Database: "mysql", System: true}, false},
		{"database operator, back over own database", dbOperator,
			backupapp.RestoreTarget{InstanceID: instA, Database: "app", Existing: ptr(ownDB)}, true},
		{"database operator, over another database", dbOperator,
			backupapp.RestoreTarget{InstanceID: instA, Database: "billing", Existing: ptr(otherDBOnA)}, false},
		{"database operator, new database", dbOperator,
			backupapp.RestoreTarget{InstanceID: instA, Database: "copy"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", nil).WithContext(withPrincipal(context.Background(), c.caller))
			err := h.authorizeRestoreTarget(r, &c.target)
			if c.allowed && err != nil {
				t.Fatalf("want allowed, got %v", err)
			}
			if !c.allowed && apperr.KindOf(err) != apperr.KindForbidden {
				t.Fatalf("want forbidden, got %v", err)
			}
		})
	}
}
