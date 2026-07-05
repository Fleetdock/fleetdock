// Package agentapp implements agent enrollment, authentication and
// heartbeats — the control-plane side of the agent protocol.
package agentapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	regtokendom "github.com/mariadb-cp/db-manager/backend/internal/domain/regtoken"
	serverdom "github.com/mariadb-cp/db-manager/backend/internal/domain/server"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
)

// ServerRepo is the server persistence surface this service needs.
type ServerRepo interface {
	serverdom.Repository
	serverdom.AgentRepository
}

// Service implements agent enrollment and liveness use cases.
type Service struct {
	servers ServerRepo
	tokens  regtokendom.Repository
}

// NewService wires the agent service.
func NewService(servers ServerRepo, tokens regtokendom.Repository) *Service {
	return &Service{servers: servers, tokens: tokens}
}

// CreateTokenInput describes a new registration token.
type CreateTokenInput struct {
	Name      string
	TTL       time.Duration
	CreatedBy *uuid.UUID
}

// CreateToken issues a single-use registration token; the raw value is
// returned exactly once.
func (s *Service) CreateToken(ctx context.Context, in CreateTokenInput) (*regtokendom.Token, string, error) {
	raw, hash, err := newToken("mdcpr")
	if err != nil {
		return nil, "", apperr.Internal(err)
	}
	ttl := in.TTL
	if ttl <= 0 || ttl > 7*24*time.Hour {
		ttl = 24 * time.Hour
	}
	t := &regtokendom.Token{
		ID:        uuid.New(),
		Name:      strings.TrimSpace(in.Name),
		TokenHash: hash,
		CreatedBy: in.CreatedBy,
		ExpiresAt: time.Now().Add(ttl),
	}
	if err := s.tokens.Create(ctx, t); err != nil {
		return nil, "", err
	}
	return t, raw, nil
}

// ListTokens returns recent registration tokens (hashes never leave the DB).
func (s *Service) ListTokens(ctx context.Context) ([]*regtokendom.Token, error) {
	return s.tokens.List(ctx)
}

// RevokeToken deletes a registration token.
func (s *Service) RevokeToken(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return apperr.Invalid("id", "id must be a valid UUID")
	}
	return s.tokens.Delete(ctx, uid)
}

// RegisterInput is what an enrolling agent sends.
type RegisterInput struct {
	Token        string
	Hostname     string
	Address      *string
	OS           *string
	AgentVersion string
}

// Register consumes a registration token, creates (or names) the server and
// issues the agent's long-lived bearer token.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*serverdom.Server, string, error) {
	if in.Token == "" || in.Hostname == "" {
		return nil, "", apperr.Invalid("token", "token and hostname are required")
	}
	rt, err := s.tokens.GetByHash(ctx, hashToken(in.Token))
	if err != nil {
		return nil, "", apperr.Unauthorized("invalid registration token")
	}
	if rt.UsedAt != nil {
		return nil, "", apperr.Unauthorized("registration token already used")
	}
	if time.Now().After(rt.ExpiresAt) {
		return nil, "", apperr.Unauthorized("registration token expired")
	}

	srv, err := s.createServer(ctx, in)
	if err != nil {
		return nil, "", err
	}
	if err := s.tokens.MarkUsed(ctx, rt.ID, srv.ID); err != nil {
		return nil, "", err
	}

	raw, hash, err := newToken("mdcpa")
	if err != nil {
		return nil, "", apperr.Internal(err)
	}
	if err := s.servers.SetAgentToken(ctx, srv.ID, hash); err != nil {
		return nil, "", err
	}
	return srv, raw, nil
}

func (s *Service) createServer(ctx context.Context, in RegisterInput) (*serverdom.Server, error) {
	name := sanitizeName(in.Hostname)
	for attempt := 0; attempt < 3; attempt++ {
		candidate := name
		if attempt > 0 {
			candidate = fmt.Sprintf("%s-%s", name, randSuffix(4))
		}
		srv, err := serverdom.NewServer(candidate, in.Hostname, in.Address, in.OS, nil, nil)
		if err != nil {
			return nil, err
		}
		err = s.servers.Create(ctx, srv)
		if err == nil {
			return srv, nil
		}
		if apperr.KindOf(err) != apperr.KindConflict {
			return nil, err
		}
	}
	return nil, apperr.Conflict("could not allocate a unique server name")
}

// Authenticate resolves the server for an agent bearer token.
func (s *Service) Authenticate(ctx context.Context, rawToken string) (*serverdom.Server, error) {
	if rawToken == "" {
		return nil, apperr.Unauthorized("agent token required")
	}
	return s.servers.GetByAgentTokenHash(ctx, hashToken(rawToken))
}

// Heartbeat records agent liveness and the latest health snapshot.
func (s *Service) Heartbeat(ctx context.Context, serverID uuid.UUID, info serverdom.HeartbeatInfo) error {
	return s.servers.Heartbeat(ctx, serverID, info)
}

// MarkStale flips servers without a recent heartbeat to offline and returns
// the ids that changed.
func (s *Service) MarkStale(ctx context.Context, olderThan time.Duration) ([]uuid.UUID, error) {
	return s.servers.MarkOffline(ctx, time.Now().Add(-olderThan))
}

// LatestServerHealth returns the current health snapshot for every server.
func (s *Service) LatestServerHealth(ctx context.Context) ([]serverdom.HealthSample, error) {
	return s.servers.LatestHealthAll(ctx)
}

// ServerMetrics returns health-history samples for one server since `since`.
func (s *Service) ServerMetrics(ctx context.Context, id string, since time.Time) ([]serverdom.HealthSample, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	if _, err := s.servers.GetByID(ctx, uid); err != nil {
		return nil, err
	}
	return s.servers.HealthHistory(ctx, uid, since)
}

// PruneHealthHistory removes health-history samples older than cutoff.
func (s *Service) PruneHealthHistory(ctx context.Context, cutoff time.Time) (int, error) {
	return s.servers.PruneHealthHistory(ctx, cutoff)
}

// ---- helpers ----

func newToken(prefix string) (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = prefix + "_" + hex.EncodeToString(buf)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randSuffix(n int) string {
	buf := make([]byte, (n+1)/2)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)[:n]
}

func sanitizeName(hostname string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(hostname) {
		switch {
		case unicode.IsLower(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case r == '.':
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-_")
	if len(name) < 2 {
		name = "server-" + randSuffix(4)
	}
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}
