// Package dbcredentialapp manages application database credentials.
package dbcredentialapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/Fleetdock/fleetdock/backend/internal/app/audit"
	"github.com/Fleetdock/fleetdock/backend/internal/app/dbtarget"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	dbcredentialdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbcredential"
	endpointdom "github.com/Fleetdock/fleetdock/backend/internal/domain/endpoint"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/conninfo"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// Secrets manages encrypted credential storage.
type Secrets interface {
	Put(ctx context.Context, ref string, kind secretdom.Kind, plaintext []byte) error
	Get(ctx context.Context, ref string) ([]byte, error)
	Delete(ctx context.Context, ref string) error
}

// Service implements credential use cases.
type Service struct {
	credentials dbcredentialdom.Repository
	databases   databasedom.Repository
	instances   instancedom.Repository
	servers     serverdom.Repository
	endpoints   endpointdom.Repository
	secrets     Secrets
	audit       *auditapp.Service
}

// NewService wires the credential service.
func NewService(credentials dbcredentialdom.Repository, databases databasedom.Repository,
	instances instancedom.Repository, servers serverdom.Repository, endpoints endpointdom.Repository,
	secrets Secrets, audit *auditapp.Service) *Service {
	return &Service{
		credentials: credentials,
		databases:   databases,
		instances:   instances,
		servers:     servers,
		endpoints:   endpoints,
		secrets:     secrets,
		audit:       audit,
	}
}

// CreateInput is the command to create an application credential.
type CreateInput struct {
	Name        string
	AccessLevel string
	Username    string
	AccountHost string
	ExpiresAt   *time.Time
	UsePublic   bool
}

// CredentialView is the API representation (no secrets by default).
type CredentialView struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Username    string     `json:"username"`
	AccessLevel string     `json:"access_level"`
	AccountHost string     `json:"account_host"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// CreateResult includes the one-time password and URL reveal.
type CreateResult struct {
	Credential    CredentialView  `json:"credential"`
	Password      string          `json:"password"`
	ConnectionURL string          `json:"connection_url"`
	CLICommand    string          `json:"cli_command,omitempty"`
	Fields        conninfo.Fields `json:"fields"`
}

// reveal assembles the one-time payload shown after create and rotate. Both
// paths must present the same shape, so they share this.
func reveal(cred *dbcredentialdom.Credential, password, database string, target endpointdom.Target) (*CreateResult, error) {
	url, err := conninfo.BuildURL(target, cred.Username, password, database)
	if err != nil {
		return nil, err
	}
	return &CreateResult{
		Credential:    toView(cred),
		Password:      password,
		ConnectionURL: url,
		CLICommand:    conninfo.CLICommand(target, cred.Username, database),
		Fields:        conninfo.BuildFields(target, cred.Username, database),
	}, nil
}

// Create provisions a DB user, stores the password encrypted, and returns a one-time reveal.
func (s *Service) Create(ctx context.Context, databaseID string, in CreateInput, actor *uuid.UUID) (*CreateResult, error) {
	db, inst, admin, conn, err := s.adminTarget(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	access := dbcredentialdom.AccessLevel(strings.TrimSpace(in.AccessLevel))
	if access == "" {
		access = dbcredentialdom.AccessReadWrite
	}
	cred, err := dbcredentialdom.NewCredential(db.ID, in.Name, in.Username, access, in.AccountHost)
	if err != nil {
		return nil, err
	}
	cred.ExpiresAt = in.ExpiresAt
	protocol, err := endpointdom.ProtocolForEngine(string(inst.Engine))
	if err != nil {
		return nil, err
	}
	// Resolve the endpoint before provisioning anything: a rejected endpoint
	// choice must not leave a DB user, a secret, and a row behind.
	ep, err := s.pickEndpoint(ctx, db.ID, in.UsePublic)
	if err != nil {
		return nil, err
	}
	password, err := genPassword()
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if err := admin.CreateDBUser(ctx, conn, cred.Username, cred.AccountHost, password); err != nil {
		return nil, apperr.FromEngine(err, "database")
	}
	profile := engine.AccessProfile(access)
	if access != dbcredentialdom.AccessCustom {
		if err := engine.ApplyProfile(ctx, admin, conn, cred.Username, cred.AccountHost, db.Name, profile); err != nil {
			_ = admin.DropDBUser(ctx, conn, cred.Username, cred.AccountHost)
			return nil, apperr.FromEngine(err, "database")
		}
	}
	cred.SecretRef = fmt.Sprintf("database/%s/credential/%s", db.ID, cred.ID)
	kind := secretdom.KindMariaDBUser
	if inst.Engine == instancedom.EnginePostgres {
		kind = secretdom.KindPostgresUser
	}
	if err := s.secrets.Put(ctx, cred.SecretRef, kind, []byte(password)); err != nil {
		_ = admin.DropDBUser(ctx, conn, cred.Username, cred.AccountHost)
		return nil, err
	}
	if err := s.credentials.Create(ctx, cred); err != nil {
		_ = s.secrets.Delete(ctx, cred.SecretRef)
		_ = admin.DropDBUser(ctx, conn, cred.Username, cred.AccountHost)
		return nil, err
	}
	out, err := reveal(cred, password, db.Name, target(ep, protocol))
	if err != nil {
		return nil, err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, actor, "credential.create", "database", &db.ID, map[string]any{
			"credential_id": cred.ID.String(), "name": cred.Name,
		})
	}
	return out, nil
}

// List returns credentials without secrets.
func (s *Service) List(ctx context.Context, databaseID string) ([]CredentialView, error) {
	dbID, err := uuid.Parse(databaseID)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	items, err := s.credentials.ListByDatabaseID(ctx, dbID)
	if err != nil {
		return nil, err
	}
	out := make([]CredentialView, 0, len(items))
	for _, c := range items {
		out = append(out, toView(c))
	}
	return out, nil
}

// Rotate replaces the password and returns a one-time reveal.
func (s *Service) Rotate(ctx context.Context, databaseID, credentialID string, actor *uuid.UUID) (*CreateResult, error) {
	db, inst, admin, conn, err := s.adminTarget(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	cred, err := s.getOwned(ctx, db.ID, credentialID)
	if err != nil {
		return nil, err
	}
	if !cred.IsActive(time.Now()) {
		return nil, apperr.Invalid("credential", "credential is not active")
	}
	protocol, err := endpointdom.ProtocolForEngine(string(inst.Engine))
	if err != nil {
		return nil, err
	}
	// Same selection rule as Create, so a rotated URL always points at the
	// endpoint the credential was issued against.
	ep, err := s.pickEndpoint(ctx, db.ID, s.hasActivePublic(ctx, db.ID))
	if err != nil {
		return nil, err
	}
	password, err := genPassword()
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if err := engine.RotatePassword(ctx, admin, conn, cred.Username, cred.AccountHost, password); err != nil {
		return nil, apperr.FromEngine(err, "database")
	}
	kind := secretdom.KindMariaDBUser
	if inst.Engine == instancedom.EnginePostgres {
		kind = secretdom.KindPostgresUser
	}
	if err := s.secrets.Put(ctx, cred.SecretRef, kind, []byte(password)); err != nil {
		return nil, err
	}
	out, err := reveal(cred, password, db.Name, target(ep, protocol))
	if err != nil {
		return nil, err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, actor, "credential.rotate", "database", &db.ID, map[string]any{
			"credential_id": cred.ID.String(),
		})
	}
	return out, nil
}

// Revoke drops the DB user and marks the credential revoked.
func (s *Service) Revoke(ctx context.Context, databaseID, credentialID string, actor *uuid.UUID) error {
	db, _, admin, conn, err := s.adminTarget(ctx, databaseID)
	if err != nil {
		return err
	}
	cred, err := s.getOwned(ctx, db.ID, credentialID)
	if err != nil {
		return err
	}
	// Must not be discarded: marking the row revoked while the database account
	// still authenticates would report success on a credential that still works.
	if err := admin.DropDBUser(ctx, conn, cred.Username, cred.AccountHost); err != nil {
		return apperr.FromEngine(err, "credential")
	}
	if err := s.credentials.Revoke(ctx, cred.ID, time.Now()); err != nil {
		return err
	}
	_ = s.secrets.Delete(ctx, cred.SecretRef)
	if s.audit != nil {
		_ = s.audit.Record(ctx, actor, "credential.revoke", "database", &db.ID, map[string]any{
			"credential_id": cred.ID.String(),
		})
	}
	return nil
}

// RevokeAllForDatabase revokes every active credential on a database.
func (s *Service) RevokeAllForDatabase(ctx context.Context, databaseID uuid.UUID, actor *uuid.UUID) error {
	items, err := s.credentials.ListByDatabaseID(ctx, databaseID)
	if err != nil {
		return err
	}
	// Revoke every credential we can, then report the ones that resisted rather
	// than stopping at the first failure — this runs while deleting a database,
	// and a single stuck account must not leave the rest untouched.
	var errs []error
	for _, c := range items {
		if c.RevokedAt != nil {
			continue
		}
		if err := s.Revoke(ctx, databaseID.String(), c.ID.String(), actor); err != nil {
			errs = append(errs, fmt.Errorf("revoke %s: %w", c.Username, err))
		}
	}
	return errors.Join(errs...)
}

// ExpireDue revokes credentials past their expiration (worker housekeeping).
func (s *Service) ExpireDue(ctx context.Context) (int, error) {
	expired, err := s.credentials.ListExpired(ctx, time.Now())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range expired {
		if err := s.Revoke(ctx, c.DatabaseID.String(), c.ID.String(), nil); err == nil {
			n++
		}
	}
	return n, nil
}

func (s *Service) adminTarget(ctx context.Context, databaseID string) (*databasedom.Database, *instancedom.Instance, engine.Admin, engine.ConnParams, error) {
	uid, err := uuid.Parse(databaseID)
	if err != nil {
		return nil, nil, nil, engine.ConnParams{}, apperr.Invalid("id", "id must be a valid UUID")
	}
	db, err := s.databases.GetByID(ctx, uid)
	if err != nil {
		return nil, nil, nil, engine.ConnParams{}, err
	}
	inst, err := s.instances.GetByID(ctx, db.InstanceID)
	if err != nil {
		return nil, nil, nil, engine.ConnParams{}, err
	}
	if !inst.HasCredentials() {
		return nil, nil, nil, engine.ConnParams{}, apperr.Invalid("instance", "instance has no admin credentials")
	}
	admin, err := engine.AdminFor(string(inst.Engine))
	if err != nil {
		return nil, nil, nil, engine.ConnParams{}, err
	}
	host, err := dbtarget.Host(ctx, s.servers, inst, "instance")
	if err != nil {
		return nil, nil, nil, engine.ConnParams{}, err
	}
	conn := engine.ConnParams{Host: host, Port: inst.Port, User: *inst.Username}
	pw, err := s.secrets.Get(ctx, *inst.RootSecretRef)
	if err != nil {
		return nil, nil, nil, engine.ConnParams{}, err
	}
	conn.Password = string(pw)
	return db, inst, admin, conn, nil
}

func (s *Service) getOwned(ctx context.Context, databaseID uuid.UUID, credentialID string) (*dbcredentialdom.Credential, error) {
	cid, err := uuid.Parse(credentialID)
	if err != nil {
		return nil, apperr.Invalid("credential_id", "credential_id must be a valid UUID")
	}
	cred, err := s.credentials.GetByID(ctx, cid)
	if err != nil {
		return nil, err
	}
	if cred.DatabaseID != databaseID {
		return nil, apperr.NotFound("credential not found")
	}
	return cred, nil
}

// hasActivePublic reports whether a usable public endpoint exists, so rotation
// can default to the same endpoint the credential was most likely created with.
func (s *Service) hasActivePublic(ctx context.Context, databaseID uuid.UUID) bool {
	ep, err := s.endpoints.GetPublicByDatabaseID(ctx, databaseID)
	return err == nil && ep.Status == endpointdom.StatusActive
}

func (s *Service) pickEndpoint(ctx context.Context, databaseID uuid.UUID, usePublic bool) (*endpointdom.Endpoint, error) {
	if usePublic {
		ep, err := s.endpoints.GetPublicByDatabaseID(ctx, databaseID)
		if err != nil {
			return nil, apperr.Invalid("public_access", "public access must be enabled to use the public endpoint")
		}
		if ep.Status != endpointdom.StatusActive {
			return nil, apperr.Invalid("public_access", "public endpoint is not active yet")
		}
		return ep, nil
	}
	db, err := s.databases.GetByID(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	return s.privateEndpoint(ctx, db)
}

func (s *Service) privateEndpoint(ctx context.Context, db *databasedom.Database) (*endpointdom.Endpoint, error) {
	inst, err := s.instances.GetByID(ctx, db.InstanceID)
	if err != nil {
		return nil, err
	}
	protocol, err := endpointdom.ProtocolForEngine(string(inst.Engine))
	if err != nil {
		return nil, err
	}
	host, err := dbtarget.Host(ctx, s.servers, inst, "instance")
	if err != nil {
		return nil, err
	}
	return endpointdom.NewPrivate(db.ID, protocol, host, inst.Port)
}

func toView(c *dbcredentialdom.Credential) CredentialView {
	return CredentialView{
		ID:          c.ID.String(),
		Name:        c.Name,
		Username:    c.Username,
		AccessLevel: string(c.AccessLevel),
		AccountHost: c.AccountHost,
		ExpiresAt:   c.ExpiresAt,
		RevokedAt:   c.RevokedAt,
		CreatedAt:   c.CreatedAt,
	}
}

func genPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// target resolves where clients connect, overriding the protocol with the one
// derived from the instance engine.
func target(ep *endpointdom.Endpoint, protocol endpointdom.Protocol) endpointdom.Target {
	t := ep.Target()
	t.Protocol = protocol
	return t
}
