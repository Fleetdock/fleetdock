package endpointapp

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	endpointdom "github.com/Fleetdock/fleetdock/backend/internal/domain/endpoint"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/gateway"
)

// fakeEndpointRepo records status writes so tests can assert what reconcile
// concluded, and can be made to fail to prove errors are not swallowed.
type fakeEndpointRepo struct {
	routable  []*endpointdom.Endpoint
	disabling []*endpointdom.Endpoint

	statusCalls []statusCall
	tlsCalls    int

	failStatus error
	failList   error
}

type statusCall struct {
	id        uuid.UUID
	status    endpointdom.Status
	lastError *string
}

func (r *fakeEndpointRepo) CreateWithPort(context.Context, *endpointdom.Endpoint, int, int) error {
	return nil
}

func (r *fakeEndpointRepo) GetPublicByDatabaseID(context.Context, uuid.UUID) (*endpointdom.Endpoint, error) {
	return nil, apperr.NotFound("public endpoint not found")
}

func (r *fakeEndpointRepo) ListRoutable(context.Context) ([]*endpointdom.Endpoint, error) {
	if r.failList != nil {
		return nil, r.failList
	}
	return r.routable, nil
}

func (r *fakeEndpointRepo) ListDisabling(context.Context) ([]*endpointdom.Endpoint, error) {
	if r.failList != nil {
		return nil, r.failList
	}
	return r.disabling, nil
}

func (r *fakeEndpointRepo) UpdateStatus(_ context.Context, id uuid.UUID, s endpointdom.Status, lastErr *string) error {
	r.statusCalls = append(r.statusCalls, statusCall{id: id, status: s, lastError: lastErr})
	return r.failStatus
}

func (r *fakeEndpointRepo) UpdateBackend(context.Context, uuid.UUID, string, int) error { return nil }

func (r *fakeEndpointRepo) UpdateTLSStatus(context.Context, uuid.UUID, endpointdom.TLSStatus) error {
	r.tlsCalls++
	return nil
}

func (r *fakeEndpointRepo) UpdateAllowedCIDRs(context.Context, uuid.UUID, []string) error { return nil }
func (r *fakeEndpointRepo) TransferDatabase(context.Context, uuid.UUID, uuid.UUID) error  { return nil }
func (r *fakeEndpointRepo) DisablePublic(context.Context, uuid.UUID) error                { return nil }

func publicEndpoint(status endpointdom.Status, port int) *endpointdom.Endpoint {
	p := port
	return &endpointdom.Endpoint{
		ID:           uuid.New(),
		DatabaseID:   uuid.New(),
		AccessType:   endpointdom.AccessPublic,
		Status:       status,
		Protocol:     endpointdom.ProtocolPostgreSQL,
		ExternalHost: "gateway.example.com",
		ExternalPort: &p,
		InternalHost: "10.0.0.5",
		InternalPort: 5432,
		TLSMode:      endpointdom.TLSRequired,
		TLSStatus:    endpointdom.TLSStatusUnknown,
		AllowedCIDRs: []string{"10.0.0.0/8"},
	}
}

// newTestService wires a service whose reloader has no config path, so Apply
// fails the way an unreachable gateway does.
func newTestService(repo *fakeEndpointRepo, gw GatewayConfig) *Service {
	return &Service{
		endpoints: repo,
		gw:        gw,
		reloader:  gateway.NewReloader(gateway.Config{ConfigPath: gw.ConfigPath, MasterSocket: gw.MasterSock}),
	}
}

// A gateway that cannot be reached must leave endpoints pending with an
// explanation. Returning early used to leave last_error nil forever, so the UI
// could never say why an endpoint never came up.
func TestReconcileRecordsApplyFailure(t *testing.T) {
	ep := publicEndpoint(endpointdom.StatusPending, 15432)
	repo := &fakeEndpointRepo{routable: []*endpointdom.Endpoint{ep}}
	// No ConfigPath: Apply fails immediately.
	svc := newTestService(repo, GatewayConfig{Enabled: true})

	err := svc.Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected the apply failure to surface")
	}
	if len(repo.statusCalls) != 1 {
		t.Fatalf("expected one status write, got %d", len(repo.statusCalls))
	}
	call := repo.statusCalls[0]
	if call.status != endpointdom.StatusPending {
		t.Errorf("status = %q, want pending", call.status)
	}
	if call.lastError == nil || *call.lastError == "" {
		t.Error("last_error must explain why the endpoint is stuck")
	}
}

// A persistent failure runs every worker tick; rewriting the same message each
// time would be a pointless write storm.
func TestReconcileDoesNotRewriteIdenticalError(t *testing.T) {
	ep := publicEndpoint(endpointdom.StatusPending, 15432)
	repo := &fakeEndpointRepo{routable: []*endpointdom.Endpoint{ep}}
	svc := newTestService(repo, GatewayConfig{Enabled: true})

	_ = svc.Reconcile(context.Background())
	if len(repo.statusCalls) != 1 {
		t.Fatalf("expected one status write, got %d", len(repo.statusCalls))
	}

	// Feed the recorded message back, as a reload from the database would.
	ep.LastError = repo.statusCalls[0].lastError
	_ = svc.Reconcile(context.Background())

	if len(repo.statusCalls) != 1 {
		t.Fatalf("unchanged error must not be rewritten, got %d writes", len(repo.statusCalls))
	}
}

// Repository failures during the status loop were discarded, so a database
// outage looked like a clean reconcile.
func TestReconcilePropagatesRepositoryErrors(t *testing.T) {
	repo := &fakeEndpointRepo{
		routable:   []*endpointdom.Endpoint{publicEndpoint(endpointdom.StatusPending, 15432)},
		failStatus: errors.New("database is down"),
	}
	svc := newTestService(repo, GatewayConfig{Enabled: true})

	if err := svc.Reconcile(context.Background()); err == nil {
		t.Fatal("a failing status write must surface")
	}
}

// Reconcile returned early when the gateway was disabled, but the worker still
// marked the job succeeded — leaving rows in "disabling" forever, which
// permanently blocked re-enabling public access.
func TestReconcileRetiresDisablingWithGatewayOff(t *testing.T) {
	ep := publicEndpoint(endpointdom.StatusDisabling, 15432)
	repo := &fakeEndpointRepo{disabling: []*endpointdom.Endpoint{ep}}
	svc := newTestService(repo, GatewayConfig{Enabled: false})

	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(repo.statusCalls) != 1 || repo.statusCalls[0].status != endpointdom.StatusDisabled {
		t.Fatalf("disabling endpoint must reach disabled, got %+v", repo.statusCalls)
	}
}

func TestReconcilePropagatesListErrors(t *testing.T) {
	repo := &fakeEndpointRepo{failList: errors.New("query failed")}
	svc := newTestService(repo, GatewayConfig{Enabled: true})

	if err := svc.Reconcile(context.Background()); err == nil {
		t.Fatal("a failing list must surface")
	}
}

func TestEnablePublicAccessRequiresGateway(t *testing.T) {
	svc := newTestService(&fakeEndpointRepo{}, GatewayConfig{Enabled: false})

	_, err := svc.EnablePublicAccess(context.Background(), uuid.NewString(), EnableInput{
		AllowedCIDRs: []string{"10.0.0.0/8"},
	}, nil)
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected an invalid error, got %v", apperr.KindOf(err))
	}
}

func TestUpdateAllowedCIDRsRequiresGateway(t *testing.T) {
	svc := newTestService(&fakeEndpointRepo{}, GatewayConfig{Enabled: false})

	_, err := svc.UpdateAllowedCIDRs(context.Background(), uuid.NewString(), []string{"10.0.0.0/8"}, nil)
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected an invalid error, got %v", apperr.KindOf(err))
	}
}

// A late reconcile must not resurrect an endpoint the user disabled.
func TestTransitionRefusesIllegalStatusChange(t *testing.T) {
	ep := publicEndpoint(endpointdom.StatusDisabled, 15432)
	repo := &fakeEndpointRepo{}
	svc := newTestService(repo, GatewayConfig{Enabled: true})

	if err := svc.transition(context.Background(), ep, endpointdom.StatusActive, nil); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if len(repo.statusCalls) != 0 {
		t.Fatalf("disabled -> active must not be written, got %+v", repo.statusCalls)
	}
}

func TestTransitionSkipsNoOpWrites(t *testing.T) {
	ep := publicEndpoint(endpointdom.StatusActive, 15432)
	repo := &fakeEndpointRepo{}
	svc := newTestService(repo, GatewayConfig{Enabled: true})

	if err := svc.transition(context.Background(), ep, endpointdom.StatusActive, nil); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if len(repo.statusCalls) != 0 {
		t.Fatalf("unchanged status must not be rewritten, got %+v", repo.statusCalls)
	}
}
