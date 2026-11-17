// Package tlsca issues the TLS certificates of managed database servers. Each
// Fleetdock install has its own certificate authority, created on first start
// and kept in the encrypted secret store; clients that want to verify the
// server (sslmode=verify-ca / verify-full, --ssl-mode=VERIFY_CA) trust its
// certificate, which GET /v1/tls/ca-certificate returns.
package tlsca

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	secretdom "github.com/Fleetdock/fleetdock/backend/internal/domain/secret"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/executor"
)

const (
	caCertRef = "tls/ca/certificate"
	caKeyRef  = "tls/ca/key"

	caValidity   = 20 * 365 * 24 * time.Hour
	leafValidity = 5 * 365 * 24 * time.Hour
)

// Secrets is the encrypted secret store (satisfied by *secretsapp.Service).
type Secrets interface {
	Get(ctx context.Context, ref string) ([]byte, error)
	Put(ctx context.Context, ref string, kind secretdom.Kind, plaintext []byte) error
}

// Servers looks up the server a managed instance runs on, for the names its
// certificate is valid for.
type Servers interface {
	GetByID(ctx context.Context, id uuid.UUID) (*serverdom.Server, error)
}

// Service is the install's certificate authority.
type Service struct {
	secrets Secrets
	servers Servers
	// extraHosts are names every server certificate also covers (the
	// gateway's public host, through which clients reach public endpoints).
	extraHosts []string
	now        func() time.Time

	mu sync.Mutex
	ca *authority
}

type authority struct {
	cert    *x509.Certificate
	certPEM string
	key     *rsa.PrivateKey
}

// NewService builds the certificate authority. extraHosts (e.g. the gateway
// public host) are added to every server certificate.
func NewService(secrets Secrets, servers Servers, extraHosts ...string) *Service {
	return &Service{secrets: secrets, servers: servers, extraHosts: extraHosts, now: time.Now}
}

// Ensure loads the authority, creating it when the install has none yet.
// Called at startup so two operations can never race to create it.
func (s *Service) Ensure(ctx context.Context) error {
	_, err := s.authority(ctx)
	return err
}

// CACertificate returns the authority's certificate (PEM).
func (s *Service) CACertificate(ctx context.Context) (string, error) {
	ca, err := s.authority(ctx)
	if err != nil {
		return "", err
	}
	return ca.certPEM, nil
}

// Issue makes a server certificate for a managed instance, valid for its
// server's address and hostname, localhost and the extra hosts.
func (s *Service) Issue(ctx context.Context, inst *instancedom.Instance) (*executor.TLSFiles, error) {
	ca, err := s.authority(ctx)
	if err != nil {
		return nil, err
	}
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if inst.ServerID != nil && s.servers != nil {
		srv, err := s.servers.GetByID(ctx, *inst.ServerID)
		if err != nil {
			return nil, fmt.Errorf("load server for certificate: %w", err)
		}
		if srv.Address != nil {
			hosts = append(hosts, *srv.Address)
		}
		hosts = append(hosts, srv.Hostname)
	}
	hosts = append(hosts, s.extraHosts...)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate server key: %w", err)
	}
	now := s.now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: inst.Name, Organization: []string{"Fleetdock"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(leafValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	addHosts(tmpl, hosts)
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("sign server certificate: %w", err)
	}
	return &executor.TLSFiles{
		CertPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		KeyPEM:   string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})),
		CAPEM:    ca.certPEM,
		NotAfter: tmpl.NotAfter.UTC().Format(time.RFC3339),
	}, nil
}

func (s *Service) authority(ctx context.Context) (*authority, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ca != nil {
		return s.ca, nil
	}
	certPEM, err := s.secrets.Get(ctx, caCertRef)
	if err == nil {
		keyPEM, err := s.secrets.Get(ctx, caKeyRef)
		if err != nil {
			return nil, fmt.Errorf("load TLS authority key: %w", err)
		}
		ca, err := parseAuthority(certPEM, keyPEM)
		if err != nil {
			return nil, err
		}
		s.ca = ca
		return ca, nil
	}
	if apperr.KindOf(err) != apperr.KindNotFound {
		return nil, fmt.Errorf("load TLS authority: %w", err)
	}

	ca, keyPEM, err := s.newAuthority()
	if err != nil {
		return nil, err
	}
	// Key first: a certificate without its key would be unusable.
	if err := s.secrets.Put(ctx, caKeyRef, secretdom.KindOther, keyPEM); err != nil {
		return nil, fmt.Errorf("store TLS authority key: %w", err)
	}
	if err := s.secrets.Put(ctx, caCertRef, secretdom.KindOther, []byte(ca.certPEM)); err != nil {
		return nil, fmt.Errorf("store TLS authority certificate: %w", err)
	}
	s.ca = ca
	return ca, nil
}

func (s *Service) newAuthority() (*authority, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, nil, fmt.Errorf("generate TLS authority key: %w", err)
	}
	now := s.now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "Fleetdock database CA", Organization: []string{"Fleetdock"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
		SubjectKeyId:          keyID(&key.PublicKey),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create TLS authority: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return &authority{cert: cert, certPEM: certPEM, key: key}, keyPEM, nil
}

func parseAuthority(certPEM, keyPEM []byte) (*authority, error) {
	cb, _ := pem.Decode(certPEM)
	kb, _ := pem.Decode(keyPEM)
	if cb == nil || kb == nil {
		return nil, errors.New("stored TLS authority is not PEM")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse TLS authority certificate: %w", err)
	}
	key, err := x509.ParsePKCS1PrivateKey(kb.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse TLS authority key: %w", err)
	}
	return &authority{cert: cert, certPEM: string(certPEM), key: key}, nil
}

// addHosts puts each host in the certificate's IP or DNS names, once.
func addHosts(c *x509.Certificate, hosts []string) {
	seen := map[string]bool{}
	for _, h := range hosts {
		h = strings.TrimSpace(strings.TrimSuffix(h, "."))
		if h == "" || seen[strings.ToLower(h)] {
			continue
		}
		seen[strings.ToLower(h)] = true
		if ip := net.ParseIP(h); ip != nil {
			c.IPAddresses = append(c.IPAddresses, ip)
		} else {
			c.DNSNames = append(c.DNSNames, h)
		}
	}
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return big.NewInt(time.Now().UnixNano())
	}
	return n
}

func keyID(pub *rsa.PublicKey) []byte {
	sum := sha256.Sum256(x509.MarshalPKCS1PublicKey(pub))
	return sum[:20]
}
