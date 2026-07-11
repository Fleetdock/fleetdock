package agentapp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	regtokendom "github.com/TajBrains/fleetdock/backend/internal/domain/regtoken"
	serverdom "github.com/TajBrains/fleetdock/backend/internal/domain/server"
	"github.com/TajBrains/fleetdock/backend/internal/platform/apperr"
)

type fakeRegTokenRepo struct {
	items map[string]*regtokendom.Token
	byID  map[uuid.UUID]*regtokendom.Token
}

func newFakeRegTokenRepo() *fakeRegTokenRepo {
	return &fakeRegTokenRepo{
		items: map[string]*regtokendom.Token{},
		byID:  map[uuid.UUID]*regtokendom.Token{},
	}
}

func (r *fakeRegTokenRepo) Create(_ context.Context, t *regtokendom.Token) error {
	r.byID[t.ID] = t
	r.items[t.TokenHash] = t
	return nil
}

func (r *fakeRegTokenRepo) List(_ context.Context) ([]*regtokendom.Token, error) {
	out := make([]*regtokendom.Token, 0, len(r.byID))
	for _, t := range r.byID {
		out = append(out, t)
	}
	return out, nil
}

func (r *fakeRegTokenRepo) Delete(_ context.Context, id uuid.UUID) error {
	t, ok := r.byID[id]
	if ok {
		delete(r.items, t.TokenHash)
		delete(r.byID, id)
	}
	return nil
}

func (r *fakeRegTokenRepo) GetByHash(_ context.Context, hash string) (*regtokendom.Token, error) {
	t, ok := r.items[hash]
	if !ok {
		return nil, apperr.NotFound("token not found")
	}
	return t, nil
}

func (r *fakeRegTokenRepo) MarkUsed(_ context.Context, id, _ uuid.UUID) error {
	t, ok := r.byID[id]
	if !ok {
		return apperr.NotFound("token not found")
	}
	now := time.Now()
	t.UsedAt = &now
	return nil
}

type fakeServerRepo struct {
	servers map[uuid.UUID]*serverdom.Server
	agents  map[string]uuid.UUID
}

func newFakeServerRepo() *fakeServerRepo {
	return &fakeServerRepo{
		servers: map[uuid.UUID]*serverdom.Server{},
		agents:  map[string]uuid.UUID{},
	}
}

func (r *fakeServerRepo) Create(_ context.Context, s *serverdom.Server) error {
	if _, exists := r.servers[s.ID]; exists {
		return apperr.Conflict("server exists")
	}
	r.servers[s.ID] = s
	return nil
}

func (r *fakeServerRepo) GetByID(_ context.Context, id uuid.UUID) (*serverdom.Server, error) {
	s, ok := r.servers[id]
	if !ok {
		return nil, apperr.NotFound("server not found")
	}
	return s, nil
}

func (r *fakeServerRepo) List(_ context.Context, _ serverdom.ListFilter) (serverdom.Page, error) {
	return serverdom.Page{}, nil
}

func (r *fakeServerRepo) SetAgentToken(_ context.Context, id uuid.UUID, hash string) error {
	r.agents[hash] = id
	return nil
}

func (r *fakeServerRepo) GetByAgentTokenHash(_ context.Context, hash string) (*serverdom.Server, error) {
	id, ok := r.agents[hash]
	if !ok {
		return nil, apperr.Unauthorized("invalid agent token")
	}
	return r.GetByID(context.Background(), id)
}

func (r *fakeServerRepo) Heartbeat(_ context.Context, id uuid.UUID, info serverdom.HeartbeatInfo) error {
	srv, ok := r.servers[id]
	if !ok {
		return apperr.NotFound("server not found")
	}
	if info.Address != nil && *info.Address != "" {
		srv.Address = info.Address
	}
	return nil
}

func (r *fakeServerRepo) MarkOffline(_ context.Context, _ time.Time) ([]uuid.UUID, error) {
	return nil, nil
}

func (r *fakeServerRepo) LatestHealthAll(_ context.Context) ([]serverdom.HealthSample, error) {
	return nil, nil
}

func (r *fakeServerRepo) HealthHistory(_ context.Context, _ uuid.UUID, _ time.Time) ([]serverdom.HealthSample, error) {
	return nil, nil
}

func (r *fakeServerRepo) PruneHealthHistory(_ context.Context, _ time.Time) (int, error) {
	return 0, nil
}

func TestCreateToken_Prefix(t *testing.T) {
	svc := NewService(newFakeServerRepo(), newFakeRegTokenRepo())

	_, raw, err := svc.CreateToken(context.Background(), CreateTokenInput{Name: "lab"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, "fleetr_") {
		t.Fatalf("expected fleetr_ prefix, got %q", raw)
	}
}

func TestRegister_RequiresFields(t *testing.T) {
	svc := NewService(newFakeServerRepo(), newFakeRegTokenRepo())

	_, _, err := svc.Register(context.Background(), RegisterInput{})
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid, got %v", apperr.KindOf(err))
	}
}

func TestRegister_Success(t *testing.T) {
	tokens := newFakeRegTokenRepo()
	servers := newFakeServerRepo()
	svc := NewService(servers, tokens)

	_, raw, err := svc.CreateToken(context.Background(), CreateTokenInput{Name: "lab"})
	if err != nil {
		t.Fatal(err)
	}

	addr := "192.168.252.2"
	srv, agentTok, err := svc.Register(context.Background(), RegisterInput{
		Token:    raw,
		Hostname: "db1.internal",
		Address:  &addr,
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if srv == nil || srv.Hostname != "db1.internal" {
		t.Fatal("expected enrolled server")
	}
	if srv.Address == nil || *srv.Address != addr {
		t.Fatalf("expected address %q, got %v", addr, srv.Address)
	}
	if !strings.HasPrefix(agentTok, "fleeta_") {
		t.Fatalf("expected fleeta_ agent token, got %q", agentTok)
	}
}

func TestHeartbeat_UpdatesAddress(t *testing.T) {
	tokens := newFakeRegTokenRepo()
	servers := newFakeServerRepo()
	svc := NewService(servers, tokens)

	_, raw, err := svc.CreateToken(context.Background(), CreateTokenInput{Name: "lab"})
	if err != nil {
		t.Fatal(err)
	}
	srv, _, err := svc.Register(context.Background(), RegisterInput{
		Token:    raw,
		Hostname: "db1.internal",
	})
	if err != nil {
		t.Fatal(err)
	}
	addr := "10.0.0.5"
	if err := svc.Heartbeat(context.Background(), srv.ID, serverdom.HeartbeatInfo{
		AgentVersion: "0.1.0",
		Address:      &addr,
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := servers.GetByID(context.Background(), srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Address == nil || *updated.Address != addr {
		t.Fatalf("expected address %q, got %v", addr, updated.Address)
	}
}

func TestRevokeToken_InvalidID(t *testing.T) {
	svc := NewService(newFakeServerRepo(), newFakeRegTokenRepo())

	err := svc.RevokeToken(context.Background(), "bad")
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid, got %v", apperr.KindOf(err))
	}
}

func TestSanitizeName(t *testing.T) {
	if got := sanitizeName("a"); got == "a" {
		t.Fatal("expected short hostname to get a generated suffix")
	}
	if got := sanitizeName("DB1.Example.COM"); !strings.HasPrefix(got, "db1-example-com") {
		t.Fatalf("unexpected sanitized name: %q", got)
	}
}
