package gateway

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"

	endpointdom "github.com/Fleetdock/fleetdock/backend/internal/domain/endpoint"
)

// startServer runs a one-shot TCP server that hands the connection to handle.
func startServer(t *testing.T, handle func(net.Conn)) (host string, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handle(conn)
	}()

	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

// postgresServer replies to an SSLRequest with the given single byte.
func postgresServer(t *testing.T, reply byte) (string, int) {
	return startServer(t, func(conn net.Conn) {
		buf := make([]byte, 8)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		_, _ = conn.Write([]byte{reply})
	})
}

func TestProbeBackendTLSPostgres(t *testing.T) {
	tests := []struct {
		name  string
		reply byte
		mode  endpointdom.TLSMode
		want  endpointdom.TLSStatus
	}{
		{"supports tls, required", 'S', endpointdom.TLSRequired, endpointdom.TLSStatusRequired},
		{"supports tls, preferred", 'S', endpointdom.TLSPreferred, endpointdom.TLSStatusPreferred},
		{"refuses tls while required", 'N', endpointdom.TLSRequired, endpointdom.TLSStatusUnsupported},
		{"refuses tls while preferred", 'N', endpointdom.TLSPreferred, endpointdom.TLSStatusDisabled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host, port := postgresServer(t, tc.reply)
			got, err := ProbeBackendTLS(context.Background(), endpointdom.ProtocolPostgreSQL, host, port, tc.mode)
			if err != nil {
				t.Fatalf("probe: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A connection that is accepted and immediately closed — exactly what an
// HAProxy allowlist rejection looks like — must be an error. Reporting it as
// "TLS unsupported" with a nil error is what let unreachable endpoints be
// marked active.
func TestProbeBackendTLSTreatsImmediateCloseAsError(t *testing.T) {
	host, port := startServer(t, func(conn net.Conn) { _ = conn.Close() })

	status, err := ProbeBackendTLS(context.Background(), endpointdom.ProtocolPostgreSQL, host, port, endpointdom.TLSRequired)
	if err == nil {
		t.Fatal("an immediately-closed connection must report an error")
	}
	if status != endpointdom.TLSStatusUnknown {
		t.Fatalf("status = %q, want unknown", status)
	}
}

func TestProbeBackendTLSUnreachable(t *testing.T) {
	// Port 1 on loopback is reliably closed.
	status, err := ProbeBackendTLS(context.Background(), endpointdom.ProtocolPostgreSQL, "127.0.0.1", 1, endpointdom.TLSRequired)
	if err == nil {
		t.Fatal("expected a dial error")
	}
	if status != endpointdom.TLSStatusUnknown {
		t.Fatalf("status = %q, want unknown", status)
	}
}

func TestProbeBackendTLSDisabledSkipsDial(t *testing.T) {
	// No server is listening; disabled mode must not attempt a connection.
	got, err := ProbeBackendTLS(context.Background(), endpointdom.ProtocolPostgreSQL, "127.0.0.1", 1, endpointdom.TLSDisabled)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got != endpointdom.TLSStatusDisabled {
		t.Fatalf("got %q, want disabled", got)
	}
}

func TestProbeBackendTLSRequiresHost(t *testing.T) {
	if _, err := ProbeBackendTLS(context.Background(), endpointdom.ProtocolPostgreSQL, "", 5432, endpointdom.TLSRequired); err == nil {
		t.Fatal("expected an error for an empty host")
	}
}

// mysqlGreeting builds a handshake packet advertising the given capabilities.
func mysqlGreeting(capabilities uint16) []byte {
	body := []byte{10} // protocol version
	body = append(body, []byte("8.0.36-test")...)
	body = append(body, 0)                      // NUL terminator
	body = append(body, 1, 0, 0, 0)             // connection id
	body = append(body, 1, 2, 3, 4, 5, 6, 7, 8) // auth-plugin-data-part-1
	body = append(body, 0)                      // filler
	body = binary.LittleEndian.AppendUint16(body, capabilities)

	packet := []byte{byte(len(body)), byte(len(body) >> 8), byte(len(body) >> 16), 0}
	return append(packet, body...)
}

func TestProbeBackendTLSMySQL(t *testing.T) {
	tests := []struct {
		name         string
		capabilities uint16
		mode         endpointdom.TLSMode
		want         endpointdom.TLSStatus
	}{
		{"advertises CLIENT_SSL", clientSSL, endpointdom.TLSRequired, endpointdom.TLSStatusRequired},
		{"advertises CLIENT_SSL, preferred", clientSSL, endpointdom.TLSPreferred, endpointdom.TLSStatusPreferred},
		{"no CLIENT_SSL while required", 0, endpointdom.TLSRequired, endpointdom.TLSStatusUnsupported},
		{"no CLIENT_SSL while preferred", 0, endpointdom.TLSPreferred, endpointdom.TLSStatusDisabled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			greeting := mysqlGreeting(tc.capabilities)
			host, port := startServer(t, func(conn net.Conn) { _, _ = conn.Write(greeting) })

			got, err := ProbeBackendTLS(context.Background(), endpointdom.ProtocolMariaDB, host, port, tc.mode)
			if err != nil {
				t.Fatalf("probe: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProbeBackendTLSMySQLErrors(t *testing.T) {
	t.Run("truncated greeting", func(t *testing.T) {
		host, port := startServer(t, func(conn net.Conn) {
			_, _ = conn.Write([]byte{50, 0, 0, 0, 10}) // claims 50 bytes, sends 1
		})
		if _, err := ProbeBackendTLS(context.Background(), endpointdom.ProtocolMySQL, host, port, endpointdom.TLSRequired); err == nil {
			t.Fatal("expected an error for a truncated greeting")
		}
	})

	t.Run("err packet", func(t *testing.T) {
		body := append([]byte{0xFF, 0x15, 0x04}, []byte("Host is not allowed to connect")...)
		packet := append([]byte{byte(len(body)), 0, 0, 0}, body...)
		host, port := startServer(t, func(conn net.Conn) { _, _ = conn.Write(packet) })

		if _, err := ProbeBackendTLS(context.Background(), endpointdom.ProtocolMySQL, host, port, endpointdom.TLSRequired); err == nil {
			t.Fatal("expected an error for an ERR packet")
		}
	})
}
