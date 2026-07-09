// Package authz holds the pure types and decision logic for per-resource
// (scoped) authorization. It has no infrastructure dependencies so it can be
// imported by both the auth service (which carries a principal's grants) and
// the authz resolver (which builds resource ancestries).
package authz

import "github.com/google/uuid"

// ScopeType is the breadth of a role grant.
type ScopeType string

const (
	// ScopeGlobal grants a permission across every resource.
	ScopeGlobal ScopeType = "global"
	// ScopeServer grants a permission on a server and everything under it
	// (its instances and their databases).
	ScopeServer ScopeType = "server"
	// ScopeDatabase grants a permission on a single database.
	ScopeDatabase ScopeType = "database"
)

// ResourceType names a kind of resource that access decisions apply to.
type ResourceType string

const (
	ResourceServer    ResourceType = "server"
	ResourceInstance  ResourceType = "instance"
	ResourceDatabase  ResourceType = "database"
	ResourceBackup    ResourceType = "backup"
	ResourceOperation ResourceType = "operation"
)

// Scope is a concrete (type, id) grant boundary. A ScopeGlobal scope has a zero
// ID and means "everything".
type Scope struct {
	Type ScopeType
	ID   uuid.UUID
}

// Grant is a single permission held at a scope — one flattened row of
// user_roles ⋈ role_permissions.
type Grant struct {
	Permission string
	Scope      Scope
}

// Ancestry is the set of concrete (non-global) scopes that confer access to a
// particular resource. For a database it is {database:D, server:S}; for an
// instance or server it is {server:S}. Callers treat global grants as universal
// and so global is never listed here.
type Ancestry struct {
	Covers []Scope
}

func (a Ancestry) covers(s Scope) bool {
	for _, c := range a.Covers {
		if c.Type == s.Type && c.ID == s.ID {
			return true
		}
	}
	return false
}

// Allow reports whether any of the grants confer perm on a resource with the
// given ancestry. A global grant of perm always allows.
func Allow(grants []Grant, perm string, anc Ancestry) bool {
	for _, g := range grants {
		if g.Permission != perm {
			continue
		}
		if g.Scope.Type == ScopeGlobal || anc.covers(g.Scope) {
			return true
		}
	}
	return false
}

// HasGlobal reports whether perm is held at global scope.
func HasGlobal(grants []Grant, perm string) bool {
	for _, g := range grants {
		if g.Permission == perm && g.Scope.Type == ScopeGlobal {
			return true
		}
	}
	return false
}

// ReadSet describes which resources a principal may read for a permission.
// When All is true there is no restriction; otherwise the caller is limited to
// the listed server- and database-scoped grants (expanded through the
// hierarchy by the query layer).
type ReadSet struct {
	All         bool
	ServerIDs   []uuid.UUID
	DatabaseIDs []uuid.UUID
}

// ReadableScope computes the read set for perm from the grants.
func ReadableScope(grants []Grant, perm string) ReadSet {
	var rs ReadSet
	for _, g := range grants {
		if g.Permission != perm {
			continue
		}
		switch g.Scope.Type {
		case ScopeGlobal:
			return ReadSet{All: true}
		case ScopeServer:
			rs.ServerIDs = append(rs.ServerIDs, g.Scope.ID)
		case ScopeDatabase:
			rs.DatabaseIDs = append(rs.DatabaseIDs, g.Scope.ID)
		}
	}
	return rs
}
