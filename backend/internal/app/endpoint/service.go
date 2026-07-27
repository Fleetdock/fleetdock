// Package endpointapp manages public database connectivity endpoints.
package endpointapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	auditapp "github.com/Fleetdock/fleetdock/backend/internal/app/audit"
	"github.com/Fleetdock/fleetdock/backend/internal/app/dbtarget"
	operationapp "github.com/Fleetdock/fleetdock/backend/internal/app/operation"
	databasedom "github.com/Fleetdock/fleetdock/backend/internal/domain/database"
	endpointdom "github.com/Fleetdock/fleetdock/backend/internal/domain/endpoint"
	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/conninfo"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/gateway"
)

// GatewayConfig holds gateway settings from application config.
type GatewayConfig struct {
	Enabled    bool
	PublicHost string
	PortStart  int
	PortEnd    int
	ConfigPath string
	MasterSock string
	// AdminSock is HAProxy's stats socket, used to read backend health and the
	// number of connections the allowlist rejected.
	AdminSock string
	// DiagPort serves the source-IP diagnostic endpoint. Zero disables it.
	DiagPort int
	// SourceIPMode is "direct" or "proxy-protocol".
	SourceIPMode string
}

// Service implements endpoint use cases.
type Service struct {
	endpoints endpointdom.Repository
	databases databasedom.Repository
	instances instancedom.Repository
	servers   serverdom.Repository
	ops       *operationapp.Service
	audit     *auditapp.Service
	gw        GatewayConfig
	reloader  *gateway.Reloader
}

// NewService wires the endpoint service.
func NewService(endpoints endpointdom.Repository, databases databasedom.Repository,
	instances instancedom.Repository, servers serverdom.Repository,
	ops *operationapp.Service, audit *auditapp.Service, gw GatewayConfig) *Service {
	return &Service{
		endpoints: endpoints,
		databases: databases,
		instances: instances,
		servers:   servers,
		ops:       ops,
		audit:     audit,
		gw:        gw,
		reloader: gateway.NewReloader(gateway.Config{
			ConfigPath:   gw.ConfigPath,
			MasterSocket: gw.MasterSock,
		}),
	}
}

// Connectivity is the combined private + public view for a database.
type Connectivity struct {
	Private *EndpointView `json:"private"`
	Public  *EndpointView `json:"public,omitempty"`
	Gateway GatewayInfo   `json:"gateway"`
}

// EndpointView is an API-safe endpoint representation.
type EndpointView struct {
	ID           string   `json:"id"`
	Status       string   `json:"status"`
	Host         string   `json:"host"`
	Port         int      `json:"port"`
	Protocol     string   `json:"protocol"`
	TLSMode      string   `json:"tls_mode"`
	TLSStatus    string   `json:"tls_status"`
	AllowedCIDRs []string `json:"allowed_cidrs,omitempty"`
	LastError    *string  `json:"last_error,omitempty"`
	// DeniedConnections counts connections the allowlist turned away. A rising
	// count with no sessions is the signature of an allowlist that does not
	// match the addresses clients actually arrive from.
	DeniedConnections int64 `json:"denied_connections"`
	SessionsTotal     int64 `json:"sessions_total"`
}

// GatewayInfo tells the UI what the deployment supports, so it can avoid
// offering public access on a control plane that has no gateway.
type GatewayInfo struct {
	Enabled      bool   `json:"enabled"`
	PublicHost   string `json:"public_host,omitempty"`
	DiagPort     int    `json:"diag_port,omitempty"`
	SourceIPMode string `json:"source_ip_mode,omitempty"`
}

// EnableInput configures public access.
type EnableInput struct {
	AllowedCIDRs   []string
	TLSMode        string
	MaxConnections *int
}

// EnableResult returns the pending endpoint and reconcile operation.
type EnableResult struct {
	Endpoint    *EndpointView `json:"endpoint"`
	OperationID string        `json:"operation_id"`
}

// GetConnectivity returns private and public endpoint metadata.
func (s *Service) GetConnectivity(ctx context.Context, databaseID string) (*Connectivity, error) {
	db, inst, err := s.loadDatabase(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	protocol, err := endpointdom.ProtocolForEngine(string(inst.Engine))
	if err != nil {
		return nil, err
	}
	internalHost, err := dbtarget.Host(ctx, s.servers, inst, "instance")
	if err != nil {
		return nil, err
	}
	out := &Connectivity{
		Private: &EndpointView{
			Status:    string(endpointdom.StatusActive),
			Host:      internalHost,
			Port:      inst.Port,
			Protocol:  string(protocol),
			TLSMode:   string(endpointdom.TLSPreferred),
			TLSStatus: string(endpointdom.TLSStatusUnknown),
		},
		Gateway: GatewayInfo{
			Enabled:      s.gw.Enabled,
			PublicHost:   s.gw.PublicHost,
			DiagPort:     s.gw.DiagPort,
			SourceIPMode: s.gw.SourceIPMode,
		},
	}

	ep, err := s.endpoints.GetPublicByDatabaseID(ctx, db.ID)
	switch {
	case err == nil:
		out.Public = s.publicView(ep)
	case apperr.KindOf(err) == apperr.KindNotFound:
		// No public endpoint is a normal state, not a failure.
	default:
		return nil, err
	}
	return out, nil
}

// publicView renders an endpoint plus whatever live counters the gateway can
// supply. Stats are best-effort: their absence must not hide the endpoint.
func (s *Service) publicView(ep *endpointdom.Endpoint) *EndpointView {
	view := toView(ep)
	if s.gw.AdminSock == "" || ep.ExternalPort == nil {
		return view
	}
	stats, err := gateway.ShowStat(s.gw.AdminSock)
	if err != nil {
		return view
	}
	if fe, ok := stats.Frontends[gateway.FrontendName(*ep.ExternalPort)]; ok {
		view.DeniedConnections = fe.DeniedConn
		view.SessionsTotal = fe.SessionsTotal
	}
	return view
}

// EnablePublicAccess allocates a port, persists a pending endpoint, and enqueues reconcile.
func (s *Service) EnablePublicAccess(ctx context.Context, databaseID string, in EnableInput, actor *uuid.UUID) (*EnableResult, error) {
	if !s.gw.Enabled {
		return nil, apperr.Invalid("gateway", "public access is not enabled on this deployment")
	}
	db, inst, err := s.loadDatabase(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	if db.Status != databasedom.StatusActive {
		return nil, apperr.Invalid("database", "database must be active to enable public access")
	}
	if _, err := s.endpoints.GetPublicByDatabaseID(ctx, db.ID); err == nil {
		return nil, apperr.Conflict("public access is already enabled for this database")
	} else if apperr.KindOf(err) != apperr.KindNotFound {
		return nil, err
	}
	internalHost, err := dbtarget.Host(ctx, s.servers, inst, "instance")
	if err != nil {
		return nil, err
	}
	protocol, err := endpointdom.ProtocolForEngine(string(inst.Engine))
	if err != nil {
		return nil, err
	}
	tlsMode := endpointdom.TLSMode(strings.TrimSpace(in.TLSMode))
	if tlsMode == "" {
		tlsMode = endpointdom.TLSRequired
	}
	if !tlsMode.Valid() {
		return nil, apperr.Invalid("tls_mode", "invalid tls mode")
	}
	// NewPublicPending normalizes the CIDRs itself; the port is assigned by
	// CreateWithPort, which allocates and inserts under one lock.
	ep, err := endpointdom.NewPublicPending(db.ID, protocol, s.gw.PublicHost, s.gw.PortStart,
		internalHost, inst.Port, in.AllowedCIDRs, tlsMode)
	if err != nil {
		return nil, err
	}
	ep.MaxConnections = in.MaxConnections
	if err := s.endpoints.CreateWithPort(ctx, ep, s.gw.PortStart, s.gw.PortEnd); err != nil {
		return nil, err
	}
	job, err := s.enqueueReconcile(ctx, &db.ID, actor)
	if err != nil {
		return nil, err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, actor, "public_access.enable", "database", &db.ID, map[string]any{
			"endpoint_id": ep.ID.String(), "port": *ep.ExternalPort,
		})
	}
	return &EnableResult{Endpoint: toView(ep), OperationID: job.ID.String()}, nil
}

// DisablePublicAccess marks the endpoint disabling and enqueues reconcile.
func (s *Service) DisablePublicAccess(ctx context.Context, databaseID string, actor *uuid.UUID) (string, error) {
	db, _, err := s.loadDatabase(ctx, databaseID)
	if err != nil {
		return "", err
	}
	ep, err := s.endpoints.GetPublicByDatabaseID(ctx, db.ID)
	if err != nil {
		return "", err
	}
	// With no gateway there is no config to reconcile, and the worker would
	// mark the job succeeded while the row sat in "disabling" forever —
	// permanently blocking re-enable.
	if !s.gw.Enabled {
		if err := s.endpoints.DisablePublic(ctx, db.ID); err != nil {
			return "", err
		}
		s.recordAudit(ctx, actor, "public_access.disable", &db.ID, ep.ID)
		return "", nil
	}

	if ep.Status != endpointdom.StatusDisabling && ep.Status != endpointdom.StatusDisabled {
		if err := s.endpoints.UpdateStatus(ctx, ep.ID, endpointdom.StatusDisabling, nil); err != nil {
			return "", err
		}
		s.recordAudit(ctx, actor, "public_access.disable", &db.ID, ep.ID)
	}
	job, err := s.enqueueReconcile(ctx, &db.ID, actor)
	if err != nil {
		return "", err
	}
	return job.ID.String(), nil
}

// UpdateAllowedCIDRs replaces the allowlist without reallocating the port, so
// existing clients keep the same address after a correction.
func (s *Service) UpdateAllowedCIDRs(ctx context.Context, databaseID string, cidrs []string, actor *uuid.UUID) (string, error) {
	if !s.gw.Enabled {
		return "", apperr.Invalid("gateway", "public access is not enabled on this deployment")
	}
	db, _, err := s.loadDatabase(ctx, databaseID)
	if err != nil {
		return "", err
	}
	ep, err := s.endpoints.GetPublicByDatabaseID(ctx, db.ID)
	if err != nil {
		return "", err
	}
	normalized, err := endpointdom.NormalizeCIDRs(cidrs)
	if err != nil {
		return "", err
	}
	if err := s.endpoints.UpdateAllowedCIDRs(ctx, ep.ID, normalized); err != nil {
		return "", err
	}
	job, err := s.enqueueReconcile(ctx, &db.ID, actor)
	if err != nil {
		return "", err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, actor, "public_access.update_cidrs", "database", &db.ID, map[string]any{
			"endpoint_id": ep.ID.String(), "allowed_cidrs": normalized,
		})
	}
	return job.ID.String(), nil
}

func (s *Service) recordAudit(ctx context.Context, actor *uuid.UUID, action string, databaseID *uuid.UUID, endpointID uuid.UUID) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, actor, action, "database", databaseID, map[string]any{
		"endpoint_id": endpointID.String(),
	})
}

// Reconcile regenerates gateway config and updates endpoint statuses.
//
// Status is derived from what the gateway actually reports, not from what the
// control plane hoped would happen:
//
//	pending — not yet present in the applied config, or the last apply failed
//	active  — in the applied config and HAProxy reports the backend reachable
//	error   — in the applied config but HAProxy reports the backend down
//
// Endpoints in error stay routed. "Error" means programmed-but-unhealthy, so a
// database restart does not tear down the listener and reallocate its port.
func (s *Service) Reconcile(ctx context.Context) error {
	eps, err := s.endpoints.ListRoutable(ctx)
	if err != nil {
		return err
	}
	disabling, err := s.endpoints.ListDisabling(ctx)
	if err != nil {
		return err
	}

	if !s.gw.Enabled {
		// Still retire disabling rows, otherwise they block re-enabling forever.
		return s.settleDisabling(ctx, disabling)
	}

	routes := make([]gateway.Route, 0, len(eps))
	for _, ep := range eps {
		if ep.ExternalPort == nil {
			continue
		}
		routes = append(routes, gateway.Route{
			ID:           ep.ID.String(),
			ListenPort:   *ep.ExternalPort,
			BackendHost:  ep.InternalHost,
			BackendPort:  ep.InternalPort,
			AllowedCIDRs: ep.AllowedCIDRs,
			MaxConn:      ep.MaxConnections,
		})
	}

	content := gateway.Generate(routes, gateway.Options{
		AdminSocket:  s.gw.AdminSock,
		DiagPort:     s.gw.DiagPort,
		SourceIPMode: s.gw.SourceIPMode,
	})
	if _, applyErr := s.reloader.Apply(content); applyErr != nil {
		// Surface the failure on the endpoints themselves. Returning early here
		// used to leave every endpoint stuck on "pending" with no last_error,
		// so the UI could never explain what went wrong.
		msg := applyErr.Error()
		errs := []error{applyErr}
		for _, ep := range eps {
			errs = append(errs, s.recordFailure(ctx, ep, msg))
		}
		return errors.Join(errs...)
	}

	var errs []error
	stats, statsErr := s.loadStats()
	for _, ep := range eps {
		errs = append(errs, s.settleRoutable(ctx, ep, stats, statsErr))
	}
	errs = append(errs, s.settleDisabling(ctx, disabling))
	return errors.Join(errs...)
}

// loadStats reads gateway runtime state. A missing admin socket is not fatal:
// endpoints simply stay pending rather than being marked healthy on no evidence.
func (s *Service) loadStats() (*gateway.Stats, error) {
	if s.gw.AdminSock == "" {
		return nil, errNoAdminSocket
	}
	return gateway.ShowStat(s.gw.AdminSock)
}

var errNoAdminSocket = errors.New("gateway admin socket is not configured; endpoint health cannot be verified")

// settleRoutable moves one endpoint to the status the gateway justifies.
func (s *Service) settleRoutable(ctx context.Context, ep *endpointdom.Endpoint,
	stats *gateway.Stats, statsErr error) error {

	if ep.ExternalPort == nil {
		return s.recordFailure(ctx, ep, "endpoint has no allocated port")
	}
	if statsErr != nil {
		return s.recordFailure(ctx, ep, statsErr.Error())
	}

	srv, ok := stats.Servers[gateway.BackendName(*ep.ExternalPort)]
	if !ok {
		return s.recordFailure(ctx, ep, "gateway has not programmed this endpoint yet")
	}
	if !srv.IsUp() {
		return s.transition(ctx, ep, endpointdom.StatusError,
			ptr(fmt.Sprintf("gateway cannot reach the database at %s:%d (%s)",
				ep.InternalHost, ep.InternalPort, strings.ToLower(srv.Status))))
	}

	var errs []error
	if tlsStatus, ok := s.probeTLS(ctx, ep); ok {
		errs = append(errs, s.endpoints.UpdateTLSStatus(ctx, ep.ID, tlsStatus))
	}
	errs = append(errs, s.transition(ctx, ep, endpointdom.StatusActive, nil))
	return errors.Join(errs...)
}

// probeTLS measures backend TLS support, skipping endpoints whose capability is
// already known. The probe opens a real database connection, so it must not run
// on every reconcile tick.
func (s *Service) probeTLS(ctx context.Context, ep *endpointdom.Endpoint) (endpointdom.TLSStatus, bool) {
	if ep.Status == endpointdom.StatusActive && ep.TLSStatus != endpointdom.TLSStatusUnknown {
		return "", false
	}
	status, err := gateway.ProbeBackendTLS(ctx, ep.Protocol, ep.InternalHost, ep.InternalPort, ep.TLSMode)
	if err != nil {
		// Reachability is the backend-health signal's job; an unusable probe
		// only means the capability stays unknown.
		return endpointdom.TLSStatusUnknown, true
	}
	return status, true
}

func (s *Service) settleDisabling(ctx context.Context, eps []*endpointdom.Endpoint) error {
	var errs []error
	for _, ep := range eps {
		errs = append(errs, s.transition(ctx, ep, endpointdom.StatusDisabled, nil))
	}
	return errors.Join(errs...)
}

// recordFailure keeps the endpoint pending and explains why, writing only when
// the message changed so a persistent failure does not rewrite the row every minute.
func (s *Service) recordFailure(ctx context.Context, ep *endpointdom.Endpoint, msg string) error {
	if ep.Status == endpointdom.StatusPending && ep.LastError != nil && *ep.LastError == msg {
		return nil
	}
	return s.transition(ctx, ep, endpointdom.StatusPending, &msg)
}

// transition applies a status change, refusing moves the lifecycle disallows so
// a late reconcile cannot resurrect a disabled endpoint.
func (s *Service) transition(ctx context.Context, ep *endpointdom.Endpoint, next endpointdom.Status, lastErr *string) error {
	if ep.Status == next && equalPtr(ep.LastError, lastErr) {
		return nil
	}
	if ep.Status != next && !ep.Status.CanTransition(next) {
		return nil
	}
	return s.endpoints.UpdateStatus(ctx, ep.ID, next, lastErr)
}

func ptr[T any](v T) *T { return &v }

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// CleanupDatabase disables endpoints and enqueues reconcile during database deletion.
func (s *Service) CleanupDatabase(ctx context.Context, databaseID uuid.UUID, actor *uuid.UUID) error {
	if err := s.endpoints.DisablePublic(ctx, databaseID); err != nil {
		return err
	}
	if s.gw.Enabled {
		_, _ = s.enqueueReconcile(ctx, &databaseID, actor)
	}
	return nil
}

// TransferOnMove moves the public endpoint to the new database record after a move.
func (s *Service) TransferOnMove(ctx context.Context, fromDatabaseID, toDatabaseID uuid.UUID, inst *instancedom.Instance, actor *uuid.UUID) error {
	ep, err := s.endpoints.GetPublicByDatabaseID(ctx, fromDatabaseID)
	if err != nil {
		if apperr.KindOf(err) == apperr.KindNotFound {
			return nil
		}
		return err
	}
	host, err := dbtarget.Host(ctx, s.servers, inst, "instance")
	if err != nil {
		return err
	}
	if err := s.endpoints.TransferDatabase(ctx, fromDatabaseID, toDatabaseID); err != nil {
		return err
	}
	if err := s.endpoints.UpdateBackend(ctx, ep.ID, host, inst.Port); err != nil {
		return err
	}
	// The endpoint now points at a different backend, so its health has to be
	// re-established rather than inherited from the old one.
	if err := s.endpoints.UpdateStatus(ctx, ep.ID, endpointdom.StatusPending, nil); err != nil {
		return err
	}
	if s.gw.Enabled {
		if _, err := s.enqueueReconcile(ctx, &toDatabaseID, actor); err != nil {
			return err
		}
	}
	return nil
}

// BuildConnectionURL generates a connection URL for the given endpoint.
func (s *Service) BuildConnectionURL(ep *endpointdom.Endpoint, protocol endpointdom.Protocol, user, password, database string) (string, error) {
	target := ep.Target()
	target.Protocol = protocol
	return conninfo.BuildURL(target, user, password, database)
}

func (s *Service) enqueueReconcile(ctx context.Context, databaseID *uuid.UUID, actor *uuid.UUID) (*jobdom.Job, error) {
	params := operationapp.Params{}
	if databaseID != nil {
		params.DatabaseID = databaseID.String()
		if db, err := s.databases.GetByID(ctx, *databaseID); err == nil {
			params.InstanceID = db.InstanceID.String()
		}
	}
	job, err := s.ops.Create(ctx, jobdom.TypeReconcileGateway, "database", databaseID, nil,
		params, actor)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Service) loadDatabase(ctx context.Context, databaseID string) (*databasedom.Database, *instancedom.Instance, error) {
	uid, err := uuid.Parse(databaseID)
	if err != nil {
		return nil, nil, apperr.Invalid("id", "id must be a valid UUID")
	}
	db, err := s.databases.GetByID(ctx, uid)
	if err != nil {
		return nil, nil, err
	}
	inst, err := s.instances.GetByID(ctx, db.InstanceID)
	if err != nil {
		return nil, nil, err
	}
	return db, inst, nil
}

func toView(ep *endpointdom.Endpoint) *EndpointView {
	port := ep.InternalPort
	if ep.ExternalPort != nil {
		port = *ep.ExternalPort
	}
	host := ep.ExternalHost
	if ep.AccessType == endpointdom.AccessPrivate {
		host = ep.InternalHost
	}
	return &EndpointView{
		ID:           ep.ID.String(),
		Status:       string(ep.Status),
		Host:         host,
		Port:         port,
		Protocol:     string(ep.Protocol),
		TLSMode:      string(ep.TLSMode),
		TLSStatus:    string(ep.TLSStatus),
		AllowedCIDRs: ep.AllowedCIDRs,
		LastError:    ep.LastError,
	}
}
