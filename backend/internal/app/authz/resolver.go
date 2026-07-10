// Package authzapp resolves the ancestry (covering scopes) of a resource so the
// HTTP layer can make per-resource authorization decisions against a
// principal's scoped grants.
package authzapp

import (
	"context"

	"github.com/google/uuid"

	authz "github.com/TajBrains/fleetdock/backend/internal/domain/authz"
)

// Repository resolves resource lineage from the metadata store.
type Repository interface {
	// ServerOfInstance returns the server id owning an instance.
	ServerOfInstance(ctx context.Context, instanceID uuid.UUID) (uuid.UUID, error)
	// LineageOfDatabase returns the instance and server ids owning a database.
	LineageOfDatabase(ctx context.Context, databaseID uuid.UUID) (instanceID, serverID uuid.UUID, err error)
	// DatabaseOfBackup returns the database id a backup belongs to.
	DatabaseOfBackup(ctx context.Context, backupID uuid.UUID) (uuid.UUID, error)
	// JobResource returns the resource_type and resource_id of a job (either may
	// be empty/nil for an unscoped job).
	JobResource(ctx context.Context, jobID uuid.UUID) (resType string, resID uuid.UUID, err error)
}

// Resolver builds resource ancestries.
type Resolver struct {
	repo Repository
}

// NewResolver wires the resolver.
func NewResolver(repo Repository) *Resolver { return &Resolver{repo: repo} }

// Ancestry returns the covering scopes for a resource. An empty ancestry means
// only a global grant can authorize access (used for unscoped jobs).
func (rv *Resolver) Ancestry(ctx context.Context, resType authz.ResourceType, resID uuid.UUID) (authz.Ancestry, error) {
	switch resType {
	case authz.ResourceServer:
		return authz.Ancestry{Covers: []authz.Scope{{Type: authz.ScopeServer, ID: resID}}}, nil

	case authz.ResourceInstance:
		serverID, err := rv.repo.ServerOfInstance(ctx, resID)
		if err != nil {
			return authz.Ancestry{}, err
		}
		return authz.Ancestry{Covers: []authz.Scope{{Type: authz.ScopeServer, ID: serverID}}}, nil

	case authz.ResourceDatabase:
		_, serverID, err := rv.repo.LineageOfDatabase(ctx, resID)
		if err != nil {
			return authz.Ancestry{}, err
		}
		return authz.Ancestry{Covers: []authz.Scope{
			{Type: authz.ScopeDatabase, ID: resID},
			{Type: authz.ScopeServer, ID: serverID},
		}}, nil

	case authz.ResourceBackup:
		dbID, err := rv.repo.DatabaseOfBackup(ctx, resID)
		if err != nil {
			return authz.Ancestry{}, err
		}
		return rv.Ancestry(ctx, authz.ResourceDatabase, dbID)

	case authz.ResourceOperation:
		rt, rid, err := rv.repo.JobResource(ctx, resID)
		if err != nil {
			return authz.Ancestry{}, err
		}
		if rt == "" || rid == uuid.Nil {
			return authz.Ancestry{}, nil // unscoped job: only a global grant allows
		}
		return rv.Ancestry(ctx, authz.ResourceType(rt), rid)
	}
	return authz.Ancestry{}, nil
}
