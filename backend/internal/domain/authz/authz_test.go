package authz

import (
	"testing"

	"github.com/google/uuid"
)

func TestAllow(t *testing.T) {
	serverA := uuid.New()
	serverB := uuid.New()
	dbX := uuid.New()

	// A database in serverA is covered by {database:dbX, server:serverA}.
	dbAnc := Ancestry{Covers: []Scope{
		{Type: ScopeDatabase, ID: dbX},
		{Type: ScopeServer, ID: serverA},
	}}

	tests := []struct {
		name   string
		grants []Grant
		perm   string
		anc    Ancestry
		want   bool
	}{
		{
			name:   "global grant allows any resource",
			grants: []Grant{{Permission: "database:write", Scope: Scope{Type: ScopeGlobal}}},
			perm:   "database:write", anc: dbAnc, want: true,
		},
		{
			name:   "server-scoped grant covers a database under that server",
			grants: []Grant{{Permission: "database:write", Scope: Scope{Type: ScopeServer, ID: serverA}}},
			perm:   "database:write", anc: dbAnc, want: true,
		},
		{
			name:   "database-scoped grant covers that database",
			grants: []Grant{{Permission: "database:write", Scope: Scope{Type: ScopeDatabase, ID: dbX}}},
			perm:   "database:write", anc: dbAnc, want: true,
		},
		{
			name:   "grant on a different server does not cover",
			grants: []Grant{{Permission: "database:write", Scope: Scope{Type: ScopeServer, ID: serverB}}},
			perm:   "database:write", anc: dbAnc, want: false,
		},
		{
			name:   "right scope but wrong permission",
			grants: []Grant{{Permission: "database:read", Scope: Scope{Type: ScopeServer, ID: serverA}}},
			perm:   "database:write", anc: dbAnc, want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Allow(tt.grants, tt.perm, tt.anc); got != tt.want {
				t.Fatalf("Allow = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasGlobal(t *testing.T) {
	sid := uuid.New()
	grants := []Grant{
		{Permission: "server:read", Scope: Scope{Type: ScopeServer, ID: sid}},
		{Permission: "database:read", Scope: Scope{Type: ScopeGlobal}},
	}
	if HasGlobal(grants, "server:read") {
		t.Fatal("server:read is only scoped, HasGlobal should be false")
	}
	if !HasGlobal(grants, "database:read") {
		t.Fatal("database:read is global, HasGlobal should be true")
	}
}

func TestReadableScope(t *testing.T) {
	serverA := uuid.New()
	dbX := uuid.New()

	// Global grant => unrestricted.
	rs := ReadableScope([]Grant{{Permission: "server:read", Scope: Scope{Type: ScopeGlobal}}}, "server:read")
	if !rs.All {
		t.Fatal("expected All for a global grant")
	}

	// Mixed scoped grants collect server and database ids.
	rs = ReadableScope([]Grant{
		{Permission: "database:read", Scope: Scope{Type: ScopeServer, ID: serverA}},
		{Permission: "database:read", Scope: Scope{Type: ScopeDatabase, ID: dbX}},
		{Permission: "database:write", Scope: Scope{Type: ScopeServer, ID: uuid.New()}}, // other perm ignored
	}, "database:read")
	if rs.All {
		t.Fatal("did not expect All")
	}
	if len(rs.ServerIDs) != 1 || rs.ServerIDs[0] != serverA {
		t.Fatalf("expected server %s, got %v", serverA, rs.ServerIDs)
	}
	if len(rs.DatabaseIDs) != 1 || rs.DatabaseIDs[0] != dbX {
		t.Fatalf("expected database %s, got %v", dbX, rs.DatabaseIDs)
	}
}
