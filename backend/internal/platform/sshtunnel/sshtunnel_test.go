package sshtunnel

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/sshtunnel/sshtunneltest"
)

// startEcho runs a line-echo server standing in for the database.
func startEcho(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); _ = c.Close() }()
		}
	}()
	return ln.Addr().String()
}

func roundTrip(t *testing.T, c net.Conn) {
	t.Helper()
	if _, err := c.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil || line != "ping\n" {
		t.Fatalf("echo = %q, %v", line, err)
	}
}

func cfgFor(s *sshtunneltest.Server) *Config {
	host, ps, _ := net.SplitHostPort(s.Addr)
	port, _ := strconv.Atoi(ps)
	return &Config{Host: host, Port: port, User: "tunnel"}
}

func TestDialPasswordPinsHostKey(t *testing.T) {
	srv := sshtunneltest.Start(t, sshtunneltest.HostKey(t), "s3cret", nil)
	db := startEcho(t)
	cfg := cfgFor(srv)
	cfg.Password = "s3cret"
	var pins []string
	cfg.OnPin = func(k string) error { pins = append(pins, k); return nil }

	c, err := Dial(context.Background(), cfg, "tcp", db)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, c)
	_ = c.Close()

	want := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(srv.HostKey.PublicKey())))
	if len(pins) != 1 || pins[0] != want {
		t.Fatalf("pins = %v, want [%s]", pins, want)
	}
	if Fingerprint(pins[0]) != ssh.FingerprintSHA256(srv.HostKey.PublicKey()) {
		t.Fatal("fingerprint mismatch")
	}
	// A second connection with the pin in place must not re-pin.
	c, err = Dial(context.Background(), cfg, "tcp", db)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	if len(pins) != 1 {
		t.Fatalf("re-pinned: %v", pins)
	}
}

func TestDialRefusesChangedHostKey(t *testing.T) {
	srv := sshtunneltest.Start(t, sshtunneltest.HostKey(t), "pw", nil)
	cfg := cfgFor(srv)
	cfg.Password = "pw"
	cfg.HostKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshtunneltest.HostKey(t).PublicKey())))

	_, err := Dial(context.Background(), cfg, "tcp", startEcho(t))
	if !errors.Is(err, ErrHostKeyMismatch) {
		t.Fatalf("err = %v, want host key mismatch", err)
	}
}

func TestDialWrongPassword(t *testing.T) {
	srv := sshtunneltest.Start(t, sshtunneltest.HostKey(t), "right", nil)
	cfg := cfgFor(srv)
	cfg.Password = "wrong"
	if _, err := Dial(context.Background(), cfg, "tcp", startEcho(t)); err == nil {
		t.Fatal("expected auth failure")
	}
}

func encryptedKey(t *testing.T, passphrase string) (string, ssh.PublicKey) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(priv, "")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block)), signer.PublicKey()
}

func TestDialPrivateKeyWithPassphrase(t *testing.T) {
	keyPEM, pub := encryptedKey(t, "open sesame")
	srv := sshtunneltest.Start(t, sshtunneltest.HostKey(t), "", pub)
	cfg := cfgFor(srv)
	cfg.PrivateKey = keyPEM
	cfg.Passphrase = "open sesame"

	c, err := Dial(context.Background(), cfg, "tcp", startEcho(t))
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, c)
	_ = c.Close()
}

func TestValidateAuth(t *testing.T) {
	enc, _ := encryptedKey(t, "pp")
	plain, _ := encryptedKey(t, "")
	cases := []struct {
		name    string
		method  string
		creds   Credentials
		wantErr string
	}{
		{"password ok", "password", Credentials{Password: "x"}, ""},
		{"password missing", "password", Credentials{}, "password is required"},
		{"key ok", "key", Credentials{PrivateKey: plain}, ""},
		{"key missing", "key", Credentials{}, "private key is required"},
		{"key garbage", "key", Credentials{PrivateKey: "nope"}, "parse private key"},
		{"encrypted no passphrase", "key", Credentials{PrivateKey: enc}, "enter its passphrase"},
		{"encrypted wrong passphrase", "key", Credentials{PrivateKey: enc, Passphrase: "bad"}, "passphrase is incorrect"},
		{"encrypted ok", "key", Credentials{PrivateKey: enc, Passphrase: "pp"}, ""},
		{"bad method", "kerberos", Credentials{}, "auth method"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAuth(tc.method, tc.creds)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestForward(t *testing.T) {
	srv := sshtunneltest.Start(t, sshtunneltest.HostKey(t), "pw", nil)
	cfg := cfgFor(srv)
	cfg.Password = "pw"
	local, closeFn, err := Forward(context.Background(), cfg, startEcho(t))
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	for range 2 {
		c, err := net.Dial("tcp", local)
		if err != nil {
			t.Fatal(err)
		}
		roundTrip(t, c)
		_ = c.Close()
	}
	closeFn()
	if c, err := net.Dial("tcp", local); err == nil {
		_ = c.Close()
		t.Fatal("listener still open after close")
	}
}

func TestCredentialsRoundTrip(t *testing.T) {
	in := Credentials{PrivateKey: "k", Passphrase: "p"}
	b, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	out, err := UnmarshalCredentials(b)
	if err != nil || out != in {
		t.Fatalf("round trip = %+v, %v", out, err)
	}
}
