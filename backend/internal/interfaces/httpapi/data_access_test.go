package httpapi

import (
	"context"
	"testing"

	"github.com/google/uuid"

	authzapp "github.com/Fleetdock/fleetdock/backend/internal/app/authz"
	dbadminapp "github.com/Fleetdock/fleetdock/backend/internal/app/dbadmin"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

type staticDataAccess dbadminapp.DataAccessInfo

func (s staticDataAccess) DataAccess(context.Context, string) (dbadminapp.DataAccessInfo, error) {
	return dbadminapp.DataAccessInfo(s), nil
}

func TestAuthorizeDataAccess(t *testing.T) {
	server := uuid.New()
	rv := authzapp.NewResolver(fakeAuthzRepo{serverID: server})
	dbUser := newScopedPrincipal("database:read", authz.Scope{Type: authz.ScopeServer, ID: server})
	instAdmin := newScopedPrincipal("instance:write", authz.Scope{Type: authz.ScopeServer, ID: server})

	cases := []struct {
		name    string
		info    dbadminapp.DataAccessInfo
		allowed bool
		admin   bool // caller is an instance administrator
	}{
		{"admin login, database user", dbadminapp.DataAccessInfo{Mode: instancedom.DataAccessAdmin}, false, false},
		{"admin login, instance admin", dbadminapp.DataAccessInfo{Mode: instancedom.DataAccessAdmin}, true, true},
		{"data login, database user", dbadminapp.DataAccessInfo{Mode: instancedom.DataAccessLogin}, true, false},
		{"managed roles, database user", dbadminapp.DataAccessInfo{Mode: instancedom.DataAccessManaged}, true, false},
		{"system database, database user", dbadminapp.DataAccessInfo{Mode: instancedom.DataAccessManaged, System: true}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := dbUser
			if c.admin {
				p = instAdmin
			}
			ctx := withPrincipal(context.Background(), p)
			c.info.InstanceID = uuid.New()
			err := authorizeDataAccess(ctx, rv, staticDataAccess(c.info), uuid.New())
			if c.allowed && err != nil {
				t.Fatalf("want allowed, got %v", err)
			}
			if !c.allowed && apperr.KindOf(err) != apperr.KindForbidden {
				t.Fatalf("want forbidden, got %v", err)
			}
		})
	}
}
