// Package sshtunnel reaches a database through an SSH bastion.
//
// Dial opens one SSH session per database connection, matching the control
// plane's no-pool model: the session is closed together with the connection.
// Forward exposes a local 127.0.0.1 port instead, for the dump/restore CLI
// tools that dial the database themselves.
//
// The bastion's host key is pinned on first use (TOFU): with no key pinned the
// presented key is accepted and reported through OnPin; afterwards any other
// key is refused until the pin is reset.
package sshtunnel

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/netsafe"
)

const handshakeTimeout = 10 * time.Second

// Credentials is the SSH secret material: a password, or a private key with
// an optional passphrase. It is stored encrypted as JSON.
type Credentials struct {
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}

// Marshal encodes credentials for secret storage.
func (c Credentials) Marshal() ([]byte, error) { return json.Marshal(c) }

// UnmarshalCredentials decodes credentials read from secret storage.
func UnmarshalCredentials(b []byte) (Credentials, error) {
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return Credentials{}, fmt.Errorf("decode ssh credentials: %w", err)
	}
	return c, nil
}

// Config describes the bastion and how to authenticate to it.
type Config struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	Credentials `json:"credentials"`
	// HostKey is the pinned host key in authorized_keys format ("" = not
	// pinned yet; the first key seen is accepted and passed to OnPin).
	HostKey string `json:"host_key,omitempty"`
	// OnPin persists the first host key seen. Errors are not fatal to the
	// connection, but are returned from Dial so they are not silently lost.
	OnPin func(key string) error `json:"-"`

	// mu guards HostKey once connections start: one Config may be shared by
	// concurrent dials (a database/sql pool), and the first one pins.
	mu sync.Mutex
}

func (c *Config) pinnedKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.HostKey
}

// ErrHostKeyMismatch is returned when the bastion presents a key other than
// the pinned one.
var ErrHostKeyMismatch = errors.New("SSH host key changed")

// Signer parses a private key, using passphrase when it is encrypted.
func Signer(privateKey, passphrase string) (ssh.Signer, error) {
	key := []byte(strings.TrimSpace(privateKey) + "\n")
	signer, err := ssh.ParsePrivateKey(key)
	var missing *ssh.PassphraseMissingError
	switch {
	case err == nil:
		return signer, nil
	case errors.As(err, &missing):
		if passphrase == "" {
			return nil, errors.New("the private key is encrypted; enter its passphrase")
		}
		signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
		if err != nil {
			if errors.Is(err, x509.IncorrectPasswordError) {
				return nil, errors.New("the private key passphrase is incorrect")
			}
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		return signer, nil
	default:
		return nil, fmt.Errorf("parse private key: %w", err)
	}
}

// ValidateAuth checks credentials are usable for the given method ("password"
// or "key") before they are saved.
func ValidateAuth(method string, c Credentials) error {
	switch method {
	case "password":
		if c.Password == "" {
			return errors.New("an SSH password is required")
		}
		return nil
	case "key":
		if strings.TrimSpace(c.PrivateKey) == "" {
			return errors.New("an SSH private key is required")
		}
		_, err := Signer(c.PrivateKey, c.Passphrase)
		return err
	}
	return errors.New("auth method must be password or key")
}

// Fingerprint returns the SHA256 fingerprint of an authorized_keys-format
// key, or "" if it cannot be parsed.
func Fingerprint(authorizedKey string) string {
	if authorizedKey == "" {
		return ""
	}
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(pk)
}

func (c *Config) addr() string { return net.JoinHostPort(c.Host, strconv.Itoa(c.port())) }

func (c *Config) port() int {
	if c.Port == 0 {
		return 22
	}
	return c.Port
}

func (c *Config) clientConfig(pinnedKey string, pinned *string) (*ssh.ClientConfig, error) {
	var auth []ssh.AuthMethod
	if strings.TrimSpace(c.PrivateKey) != "" {
		signer, err := Signer(c.PrivateKey, c.Passphrase)
		if err != nil {
			return nil, err
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if c.Password != "" {
		pw := c.Password
		auth = append(auth, ssh.Password(pw),
			ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
				answers := make([]string, len(qs))
				for i := range answers {
					answers[i] = pw
				}
				return answers, nil
			}))
	}
	if len(auth) == 0 {
		return nil, errors.New("no SSH credentials configured")
	}
	return &ssh.ClientConfig{
		User: c.User,
		Auth: auth,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			presented := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
			if pinnedKey == "" {
				*pinned = presented
				return nil
			}
			want, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pinnedKey))
			if err != nil {
				return fmt.Errorf("pinned SSH host key is invalid: %w", err)
			}
			if string(want.Marshal()) != string(key.Marshal()) {
				return fmt.Errorf("%w (now %s, pinned %s); reset the host key on the instance if this change is expected",
					ErrHostKeyMismatch, ssh.FingerprintSHA256(key), ssh.FingerprintSHA256(want))
			}
			return nil
		},
		Timeout: handshakeTimeout,
	}, nil
}

// connect dials the bastion (under the netsafe database policy, so the SSH
// host gets the same protection a direct database host would) and completes
// the SSH handshake.
func (c *Config) connect(ctx context.Context) (*ssh.Client, error) {
	var pinned string
	cfg, err := c.clientConfig(c.pinnedKey(), &pinned)
	if err != nil {
		return nil, err
	}
	hctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	raw, err := netsafe.DialDB(hctx, "tcp", c.addr())
	if err != nil {
		return nil, fmt.Errorf("ssh tunnel: connect to %s: %w", c.addr(), err)
	}
	if dl, ok := hctx.Deadline(); ok {
		_ = raw.SetDeadline(dl)
	}
	sc, chans, reqs, err := ssh.NewClientConn(raw, c.addr(), cfg)
	if err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("ssh tunnel: %w", err)
	}
	_ = raw.SetDeadline(time.Time{})
	client := ssh.NewClient(sc, chans, reqs)
	if pinned != "" {
		if err := c.pin(pinned); err != nil {
			_ = client.Close()
			return nil, err
		}
	}
	return client, nil
}

// pin records the first key seen. A concurrent dial may have pinned first;
// then the key it saw must be the one this dial saw too.
func (c *Config) pin(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.HostKey != "" {
		if c.HostKey != key {
			return fmt.Errorf("%w during first connection; reset the host key on the instance and retry", ErrHostKeyMismatch)
		}
		return nil
	}
	if c.OnPin != nil {
		if err := c.OnPin(key); err != nil {
			return fmt.Errorf("ssh tunnel: pin host key: %w", err)
		}
	}
	c.HostKey = key
	return nil
}

// Dial opens a connection to dbAddr as seen from the bastion. Closing the
// returned connection also closes its SSH session.
func Dial(ctx context.Context, c *Config, network, dbAddr string) (net.Conn, error) {
	client, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	conn, err := client.DialContext(ctx, network, dbAddr)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ssh tunnel: bastion could not reach %s: %w", dbAddr, err)
	}
	return &tunnelConn{Conn: conn, client: client}, nil
}

type tunnelConn struct {
	net.Conn
	client *ssh.Client
	once   sync.Once
}

func (t *tunnelConn) Close() error {
	err := t.Conn.Close()
	t.once.Do(func() { _ = t.client.Close() })
	return err
}

// Forward listens on an ephemeral 127.0.0.1 port and forwards every accepted
// connection to dbAddr through one SSH session. The returned close function
// stops the listener and tears the session down.
func Forward(ctx context.Context, c *Config, dbAddr string) (string, func(), error) {
	client, err := c.connect(ctx)
	if err != nil {
		return "", nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = client.Close()
		return "", nil, fmt.Errorf("ssh tunnel: local listener: %w", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			local, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { _ = local.Close() }()
				remote, err := client.Dial("tcp", dbAddr)
				if err != nil {
					return
				}
				defer func() { _ = remote.Close() }()
				pipe(local, remote)
			}()
		}
	}()
	var once sync.Once
	closeFn := func() {
		once.Do(func() {
			_ = ln.Close()
			_ = client.Close()
			wg.Wait()
		})
	}
	return ln.Addr().String(), closeFn, nil
}

// pipe copies both ways until either side finishes, then unblocks the other.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	_ = a.Close()
	_ = b.Close()
	<-done
}
