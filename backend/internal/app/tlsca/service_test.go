package tlsca

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/google/uuid"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

type memSecrets map[string][]byte

func (m memSecrets) Get(_ context.Context, ref string) ([]byte, error) {
	v, ok := m[ref]
	if !ok {
		return nil, apperr.NotFound("secret not found")
	}
	return v, nil
}

func (m memSecrets) Put(_ context.Context, ref string, _ secretdom.Kind, b []byte) error {
	m[ref] = b
	return nil
}

type oneServer struct{ srv *serverdom.Server }

func (o oneServer) GetByID(context.Context, uuid.UUID) (*serverdom.Server, error) { return o.srv, nil }

func TestIssue_CertificateVerifiesAgainstTheAuthority(t *testing.T) {
	addr := "10.0.0.5"
	sid := uuid.New()
	secrets := memSecrets{}
	svc := NewService(secrets, oneServer{&serverdom.Server{ID: sid, Hostname: "db-1", Address: &addr}}, "gw.example.com")

	files, err := svc.Issue(context.Background(), &instancedom.Instance{ID: uuid.New(), Name: "orders", ServerID: &sid})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := tls.X509KeyPair([]byte(files.CertPEM), []byte(files.KeyPEM)); err != nil {
		t.Fatalf("certificate and key don't match: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(files.CAPEM)) {
		t.Fatal("CA PEM did not parse")
	}
	block, _ := pem.Decode([]byte(files.CertPEM))
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"10.0.0.5", "db-1", "gw.example.com", "localhost", "127.0.0.1"} {
		if _, err := cert.Verify(x509.VerifyOptions{Roots: pool, DNSName: host}); err != nil {
			t.Errorf("verify for %s: %v", host, err)
		}
	}
	if files.NotAfter == "" {
		t.Error("NotAfter is empty")
	}
}

func TestAuthority_IsKeptAndReloaded(t *testing.T) {
	secrets := memSecrets{}
	first := NewService(secrets, nil)
	if err := first.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	ca1, _ := first.CACertificate(context.Background())

	second := NewService(secrets, nil)
	ca2, err := second.CACertificate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ca1 != ca2 {
		t.Error("a second service created a new authority instead of loading the stored one")
	}
}
