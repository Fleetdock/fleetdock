package operationapp

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	jobdom "github.com/Fleetdock/fleetdock/backend/internal/domain/job"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/executor"
)

type tlsInstances struct {
	instancedom.Repository
	inst   *instancedom.Instance
	tlsSet *time.Time
}

func (r *tlsInstances) GetByID(context.Context, uuid.UUID) (*instancedom.Instance, error) {
	return r.inst, nil
}

func (r *tlsInstances) SetContainerID(context.Context, uuid.UUID, string) error { return nil }

func (r *tlsInstances) SetTLS(_ context.Context, _ uuid.UUID, at time.Time) error {
	r.tlsSet = &at
	return nil
}

type fakeIssuer struct{ calls int }

func (f *fakeIssuer) Issue(context.Context, *instancedom.Instance) (*executor.TLSFiles, error) {
	f.calls++
	return &executor.TLSFiles{CertPEM: "C", KeyPEM: "K", CAPEM: "A", NotAfter: "2031-01-01T00:00:00Z"}, nil
}

func tlsService(inst *instancedom.Instance) (*Service, *tlsInstances, *fakeIssuer) {
	repo := &tlsInstances{inst: inst}
	svc := NewService(newFakeJobRepo(), repo, nil, nil, nil, nil)
	issuer := &fakeIssuer{}
	svc.SetTLSIssuer(issuer)
	return svc, repo, issuer
}

func TestProvisionPayload_IssuesACertificateOnlyWithTLS(t *testing.T) {
	inst := &instancedom.Instance{ID: uuid.New(), Engine: instancedom.EnginePostgres, EngineVersion: "17", Port: 5433}
	svc, _, issuer := tlsService(inst)

	p, err := svc.provisionPayload(context.Background(), jobdom.TypeProvisionInstance, Params{InstanceID: inst.ID.String(), Version: "17"})
	if err != nil || p.Provision.TLS != nil || issuer.calls != 0 {
		t.Fatalf("without TLS: payload TLS = %v, calls = %d, err = %v", p.Provision.TLS, issuer.calls, err)
	}
	p, err = svc.provisionPayload(context.Background(), jobdom.TypeProvisionInstance, Params{InstanceID: inst.ID.String(), Version: "17", TLS: true})
	if err != nil || p.Provision.TLS == nil {
		t.Fatalf("with TLS: payload TLS = %v, err = %v", p.Provision.TLS, err)
	}
}

func TestConfigureTLS_PayloadAndCompletion(t *testing.T) {
	inst := &instancedom.Instance{ID: uuid.New(), Engine: instancedom.EngineMySQL, EngineVersion: "8.4", Port: 3307}
	svc, repo, _ := tlsService(inst)

	p, err := svc.provisionPayload(context.Background(), jobdom.TypeConfigureTLS, Params{InstanceID: inst.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if p.Provision.TLS == nil || p.Provision.Version != "8.4" || p.Provision.Port != 3307 || p.Provision.Image != "" {
		t.Errorf("configure_tls spec = %+v", p.Provision)
	}

	svc.completeProvision(context.Background(), &jobdom.Job{Type: jobdom.TypeConfigureTLS, ResourceID: &inst.ID}, true,
		[]byte(`{"ok":true,"container_id":"abc","tls_not_after":"2031-01-01T00:00:00Z"}`), nil)
	if repo.tlsSet == nil || repo.tlsSet.Year() != 2031 {
		t.Errorf("TLS expiry not recorded: %v", repo.tlsSet)
	}
}
