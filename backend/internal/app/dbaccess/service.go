// Package dbaccessapp provisions the least-privilege database roles that
// interactive features (SQL console, table browser, CSV export, data and
// structure editing) connect as.
//
// Root credentials stay reserved for instance-level administration. Every
// per-database interactive request instead runs as fleetdock_ro_<id> or
// fleetdock_rw_<id>, which the engine itself confines to that one database:
// another database, the account tables, server files and global settings are
// out of reach no matter what SQL is typed.
package dbaccessapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	dbaccessdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbaccess"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// Secrets is the secret store surface this service needs.
type Secrets interface {
	Put(ctx context.Context, ref string, kind secretdom.Kind, plaintext []byte) error
	Get(ctx context.Context, ref string) ([]byte, error)
	Delete(ctx context.Context, ref string) error
}

// Target is everything needed to reach a database as its instance admin.
type Target struct {
	Instance *instancedom.Instance
	Database *databasedom.Database
	Admin    engine.Admin
	Root     engine.ConnParams
}

// Service provisions and resolves console roles.
type Service struct {
	roles   dbaccessdom.Repository
	secrets Secrets

	mu       sync.Mutex
	inflight map[string]*sync.Mutex // one provisioning at a time per role
}

// NewService wires the service.
func NewService(roles dbaccessdom.Repository, secrets Secrets) *Service {
	return &Service{roles: roles, secrets: secrets, inflight: map[string]*sync.Mutex{}}
}

// accountHost is the MySQL/MariaDB account host for console roles. The
// control plane's egress address is not knowable in general (NAT, proxies),
// and the password is a 48-character random secret that never leaves the
// control plane, so any host is accepted.
const accountHost = "%"

// Username is the deterministic role name for a database and mode. Short
// enough for MySQL's 32-character limit and valid as an unquoted identifier.
func Username(db *databasedom.Database, mode dbaccessdom.Mode) string {
	id := strings.ReplaceAll(db.ID.String(), "-", "")
	return fmt.Sprintf("fleetdock_%s_%s", mode, id[:12])
}

func secretRef(db *databasedom.Database, mode dbaccessdom.Mode) string {
	return "dbaccess/" + db.ID.String() + "/" + string(mode)
}

func secretKind(inst *instancedom.Instance) secretdom.Kind {
	if inst.Engine == instancedom.EnginePostgres {
		return secretdom.KindPostgresUser
	}
	return secretdom.KindMariaDBUser
}

// Conn returns connection parameters for the database's console role of the
// given mode, provisioning the role on first use.
func (s *Service) Conn(ctx context.Context, t Target, mode dbaccessdom.Mode) (engine.ConnParams, error) {
	role, err := s.roles.Get(ctx, t.Database.ID, mode)
	if err != nil && apperr.KindOf(err) != apperr.KindNotFound {
		return engine.ConnParams{}, err
	}
	if role == nil {
		if role, err = s.provision(ctx, t, mode); err != nil {
			return engine.ConnParams{}, err
		}
	}
	pw, err := s.secrets.Get(ctx, role.SecretRef)
	if err != nil {
		return engine.ConnParams{}, apperr.Internal(fmt.Errorf("load access role secret: %w", err))
	}
	conn := t.Root
	conn.User = role.Username
	conn.Password = string(pw)
	conn.Database = t.Database.Name
	conn.AssumeOwner = mode == dbaccessdom.ModeWrite
	return conn, nil
}

// lockFor serialises provisioning of one role across concurrent requests.
func (s *Service) lockFor(key string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.inflight[key]
	if !ok {
		m = &sync.Mutex{}
		s.inflight[key] = m
	}
	return m
}

// provision creates the engine account, grants it the profile and records it.
// If an account of that name already exists (e.g. metadata was restored from
// an older backup) it is dropped and recreated so its password is known.
func (s *Service) provision(ctx context.Context, t Target, mode dbaccessdom.Mode) (*dbaccessdom.Role, error) {
	key := t.Database.ID.String() + "/" + string(mode)
	l := s.lockFor(key)
	l.Lock()
	defer l.Unlock()

	// Another request may have finished provisioning while we waited.
	if role, err := s.roles.Get(ctx, t.Database.ID, mode); err == nil {
		return role, nil
	}

	user := Username(t.Database, mode)
	password, err := genPassword()
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if err := t.Admin.CreateDBUser(ctx, t.Root, user, accountHost, password); err != nil {
		// Most likely the account exists from an earlier, forgotten
		// provisioning. Replace it.
		if derr := t.Admin.DropDBUser(ctx, t.Root, user, accountHost); derr != nil {
			return nil, apperr.FromEngine(fmt.Errorf("create access role: %w", err), "database")
		}
		if err := t.Admin.CreateDBUser(ctx, t.Root, user, accountHost, password); err != nil {
			return nil, apperr.FromEngine(fmt.Errorf("create access role: %w", err), "database")
		}
	}
	if err := engine.ApplyConsoleProfile(ctx, t.Admin, t.Root, user, accountHost, t.Database.Name, mode == dbaccessdom.ModeWrite); err != nil {
		_ = t.Admin.DropDBUser(ctx, t.Root, user, accountHost)
		return nil, apperr.FromEngine(fmt.Errorf("grant access role: %w", err), "database")
	}

	ref := secretRef(t.Database, mode)
	if err := s.secrets.Put(ctx, ref, secretKind(t.Instance), []byte(password)); err != nil {
		_ = t.Admin.DropDBUser(ctx, t.Root, user, accountHost)
		return nil, err
	}
	role := &dbaccessdom.Role{DatabaseID: t.Database.ID, Mode: mode, Username: user, SecretRef: ref}
	if err := s.roles.Upsert(ctx, role); err != nil {
		_ = t.Admin.DropDBUser(ctx, t.Root, user, accountHost)
		return nil, err
	}
	slog.Info("provisioned database access role", "database_id", t.Database.ID, "mode", mode, "user", user)
	return role, nil
}

// Reapply re-runs the role's grants. Grants are snapshots on PostgreSQL —
// tables and schemas created later by other roles are not covered — so the
// caller invokes this when the role hits "permission denied", then retries.
// A role that was never provisioned is provisioned instead.
func (s *Service) Reapply(ctx context.Context, t Target, mode dbaccessdom.Mode) error {
	role, err := s.roles.Get(ctx, t.Database.ID, mode)
	if err != nil {
		if apperr.KindOf(err) == apperr.KindNotFound {
			_, err = s.provision(ctx, t, mode)
		}
		return err
	}
	if err := engine.ApplyConsoleProfile(ctx, t.Admin, t.Root, role.Username, accountHost, t.Database.Name, mode == dbaccessdom.ModeWrite); err != nil {
		return apperr.FromEngine(fmt.Errorf("re-grant access role: %w", err), "database")
	}
	return s.roles.Touch(ctx, t.Database.ID, mode)
}

// Drop removes a database's console roles from the engine and forgets them.
// Engine failures are logged, not returned: the database itself is usually
// already gone, and a leftover role with no grants is harmless.
func (s *Service) Drop(ctx context.Context, t Target) error {
	roles, err := s.roles.ListByDatabase(ctx, t.Database.ID)
	if err != nil {
		return err
	}
	for _, role := range roles {
		if t.Admin != nil {
			if err := t.Admin.DropDBUser(ctx, t.Root, role.Username, accountHost); err != nil {
				slog.Warn("drop database access role", "user", role.Username, "error", err.Error())
			}
		}
		_ = s.secrets.Delete(ctx, role.SecretRef)
	}
	return s.roles.DeleteByDatabase(ctx, t.Database.ID)
}

func genPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
