package dbaccessapp

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/google/uuid"

	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	dbaccessdom "github.com/Fleetdock/fleetdock/backend/internal/domain/dbaccess"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

// fakeAdmin records account and grant calls.
type fakeAdmin struct {
	engine.Admin // unused methods panic
	mu           sync.Mutex
	users        map[string]string // user -> password
	profiles     map[string]engine.AccessProfile
	creates      int
	failCreate   bool
}

func newFakeAdmin() *fakeAdmin {
	return &fakeAdmin{users: map[string]string{}, profiles: map[string]engine.AccessProfile{}}
}

func (a *fakeAdmin) CreateDBUser(_ context.Context, _ engine.ConnParams, user, _, pw string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.failCreate {
		return errors.New("boom")
	}
	if _, ok := a.users[user]; ok {
		return errors.New("user exists")
	}
	a.creates++
	a.users[user] = pw
	return nil
}

func (a *fakeAdmin) DropDBUser(_ context.Context, _ engine.ConnParams, user, _ string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.users, user)
	return nil
}

func (a *fakeAdmin) ApplyAccessProfile(_ context.Context, _ engine.ConnParams, user, _, _ string, prof engine.AccessProfile) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.profiles[user] = prof
	return nil
}

func (a *fakeAdmin) ExportCSV(context.Context, engine.ConnParams, string, string, string, io.Writer, func()) (int64, error) {
	return 0, nil
}

type memRoles struct {
	mu    sync.Mutex
	items map[string]*dbaccessdom.Role
}

func key(id uuid.UUID, m dbaccessdom.Mode) string { return id.String() + string(m) }

func (r *memRoles) Get(_ context.Context, id uuid.UUID, m dbaccessdom.Mode) (*dbaccessdom.Role, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if role, ok := r.items[key(id, m)]; ok {
		c := *role
		return &c, nil
	}
	return nil, apperr.NotFound("no role")
}
func (r *memRoles) Upsert(_ context.Context, role *dbaccessdom.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *role
	r.items[key(role.DatabaseID, role.Mode)] = &c
	return nil
}
func (r *memRoles) Touch(context.Context, uuid.UUID, dbaccessdom.Mode) error { return nil }
func (r *memRoles) ListByDatabase(_ context.Context, id uuid.UUID) ([]*dbaccessdom.Role, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*dbaccessdom.Role
	for _, role := range r.items {
		if role.DatabaseID == id {
			out = append(out, role)
		}
	}
	return out, nil
}
func (r *memRoles) DeleteByDatabase(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, role := range r.items {
		if role.DatabaseID == id {
			delete(r.items, k)
		}
	}
	return nil
}

type memSecrets struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (s *memSecrets) Put(_ context.Context, ref string, _ secretdom.Kind, p []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[ref] = p
	return nil
}
func (s *memSecrets) Get(_ context.Context, ref string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[ref]
	if !ok {
		return nil, apperr.NotFound("no secret")
	}
	return v, nil
}
func (s *memSecrets) Delete(_ context.Context, ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, ref)
	return nil
}

func fixture() (*Service, *fakeAdmin, *memSecrets, Target) {
	admin := newFakeAdmin()
	secrets := &memSecrets{m: map[string][]byte{}}
	svc := NewService(&memRoles{items: map[string]*dbaccessdom.Role{}}, secrets)
	t := Target{
		Instance: &instancedom.Instance{ID: uuid.New(), Engine: instancedom.EngineMariaDB},
		Database: &databasedom.Database{ID: uuid.New(), Name: "app"},
		Admin:    admin,
		Root:     engine.ConnParams{Host: "db", Port: 3306, User: "root", Password: "rootpw"},
	}
	return svc, admin, secrets, t
}

func TestConn_ProvisionsOnceAndNeverUsesRoot(t *testing.T) {
	svc, admin, _, tgt := fixture()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := svc.Conn(ctx, tgt, dbaccessdom.ModeRead)
			if err != nil {
				t.Errorf("Conn: %v", err)
				return
			}
			if conn.User == "root" || conn.Password == "rootpw" {
				t.Errorf("console connection must not use root credentials: %+v", conn)
			}
			if conn.Database != "app" || conn.Host != "db" {
				t.Errorf("conn = %+v, want host db, database app", conn)
			}
		}()
	}
	wg.Wait()

	if admin.creates != 1 {
		t.Fatalf("role created %d times, want exactly once", admin.creates)
	}
	user := Username(tgt.Database, dbaccessdom.ModeRead)
	if admin.profiles[user] != engine.ProfileReadonly {
		t.Errorf("read role profile = %q, want readonly", admin.profiles[user])
	}
}

func TestConn_WriteRoleGetsReadWrite(t *testing.T) {
	svc, admin, _, tgt := fixture()
	conn, err := svc.Conn(context.Background(), tgt, dbaccessdom.ModeWrite)
	if err != nil {
		t.Fatal(err)
	}
	if admin.profiles[conn.User] != engine.ProfileReadWrite {
		t.Errorf("write role profile = %q, want readwrite", admin.profiles[conn.User])
	}
	if conn.User == Username(tgt.Database, dbaccessdom.ModeRead) {
		t.Error("read and write roles must differ")
	}
}

func TestConn_ReplacesLeftoverAccount(t *testing.T) {
	svc, admin, _, tgt := fixture()
	admin.users[Username(tgt.Database, dbaccessdom.ModeRead)] = "unknown-old-password"

	conn, err := svc.Conn(context.Background(), tgt, dbaccessdom.ModeRead)
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	if admin.users[conn.User] != conn.Password {
		t.Error("leftover account should be recreated with a known password")
	}
}

func TestDrop_RemovesRolesAndSecrets(t *testing.T) {
	svc, admin, secrets, tgt := fixture()
	ctx := context.Background()
	if _, err := svc.Conn(ctx, tgt, dbaccessdom.ModeRead); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Conn(ctx, tgt, dbaccessdom.ModeWrite); err != nil {
		t.Fatal(err)
	}
	if err := svc.Drop(ctx, tgt); err != nil {
		t.Fatal(err)
	}
	if len(admin.users) != 0 || len(secrets.m) != 0 {
		t.Errorf("after drop: users=%v secrets=%d, want none", admin.users, len(secrets.m))
	}
}

func TestUsernameFitsMySQLLimit(t *testing.T) {
	db := &databasedom.Database{ID: uuid.New()}
	for _, m := range []dbaccessdom.Mode{dbaccessdom.ModeRead, dbaccessdom.ModeWrite} {
		if u := Username(db, m); len(u) > 32 {
			t.Errorf("%s is %d chars; MySQL allows 32", u, len(u))
		}
	}
}
