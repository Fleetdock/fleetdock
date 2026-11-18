package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	authzapp "github.com/Fleetdock/fleetdock/backend/internal/app/authz"
	dbadminapp "github.com/Fleetdock/fleetdock/backend/internal/app/dbadmin"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

func TestAuthorizeSystemDatabase(t *testing.T) {
	server := uuid.New()
	rv := authzapp.NewResolver(fakeAuthzRepo{serverID: server})
	dbWriter := newScopedPrincipal("database:write", authz.Scope{Type: authz.ScopeServer, ID: server})
	instAdmin := newScopedPrincipal("instance:write", authz.Scope{Type: authz.ScopeServer, ID: server})

	for _, c := range []struct {
		name    string
		info    dbadminapp.DataAccessInfo
		admin   bool
		allowed bool
	}{
		{"system database, database writer", dbadminapp.DataAccessInfo{System: true}, false, false},
		{"system database, instance admin", dbadminapp.DataAccessInfo{System: true}, true, true},
		{"application database, database writer", dbadminapp.DataAccessInfo{}, false, true},
		// Credentials and grants create accounts scoped to the database; the
		// instance's data access login does not matter for them.
		{"admin data login, database writer", dbadminapp.DataAccessInfo{Mode: instancedom.DataAccessAdmin}, false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := dbWriter
			if c.admin {
				p = instAdmin
			}
			c.info.InstanceID = uuid.New()
			err := authorizeSystemDatabase(withPrincipal(context.Background(), p), rv, staticDataAccess(c.info), uuid.NewString())
			if c.allowed && err != nil {
				t.Fatalf("want allowed, got %v", err)
			}
			if !c.allowed && apperr.KindOf(err) != apperr.KindForbidden {
				t.Fatalf("want forbidden, got %v", err)
			}
		})
	}
}

// The credential routes refuse a system database before the service runs (it
// is nil here, so reaching it would panic).
func TestDBCredentialsRefuseSystemDatabase(t *testing.T) {
	server := uuid.New()
	rv := authzapp.NewResolver(fakeAuthzRepo{serverID: server})
	dbWriter := newScopedPrincipal("database:write", authz.Scope{Type: authz.ScopeServer, ID: server})
	h := NewDBCredentialHandler(nil, rv, staticDataAccess(dbadminapp.DataAccessInfo{InstanceID: uuid.New(), System: true}))

	dbID := uuid.NewString()
	for name, call := range map[string]func(w http.ResponseWriter, r *http.Request){
		"create": h.Create,
		"rotate": h.Rotate,
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"app","access_level":"readonly"}`))
			r.SetPathValue("id", dbID)
			r.SetPathValue("credentialId", uuid.NewString())
			r = r.WithContext(withPrincipal(r.Context(), dbWriter))
			w := httptest.NewRecorder()
			call(w, r)
			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403: %s", w.Code, w.Body)
			}
		})
	}
}
