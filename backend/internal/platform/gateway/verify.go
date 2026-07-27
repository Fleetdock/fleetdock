package gateway

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	endpointdom "github.com/Fleetdock/fleetdock/backend/internal/domain/endpoint"
)

const (
	probeDialTimeout = 8 * time.Second
	probeReadTimeout = 5 * time.Second
)

// ProbeBackendTLS reports whether the database itself accepts TLS.
//
// It dials the backend directly rather than going through HAProxy: the gateway
// enforces the endpoint's CIDR allowlist, which the control plane is not a
// member of, so a probe through the public port only ever measures the ACL.
func ProbeBackendTLS(ctx context.Context, protocol endpointdom.Protocol, host string, port int,
	mode endpointdom.TLSMode) (endpointdom.TLSStatus, error) {

	if host == "" {
		return endpointdom.TLSStatusUnknown, fmt.Errorf("endpoint has no backend host")
	}
	if mode == endpointdom.TLSDisabled {
		return endpointdom.TLSStatusDisabled, nil
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	d := net.Dialer{Timeout: probeDialTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return endpointdom.TLSStatusUnknown, fmt.Errorf("dial backend %s: %w", addr, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(probeReadTimeout)); err != nil {
		return endpointdom.TLSStatusUnknown, err
	}

	var supportsTLS bool
	switch protocol {
	case endpointdom.ProtocolPostgreSQL:
		supportsTLS, err = postgresSupportsTLS(conn)
	case endpointdom.ProtocolMySQL, endpointdom.ProtocolMariaDB:
		supportsTLS, err = mysqlSupportsTLS(conn)
	default:
		return endpointdom.TLSStatusUnknown, nil
	}
	if err != nil {
		return endpointdom.TLSStatusUnknown, err
	}

	if !supportsTLS {
		if mode == endpointdom.TLSRequired {
			return endpointdom.TLSStatusUnsupported, nil
		}
		return endpointdom.TLSStatusDisabled, nil
	}
	if mode == endpointdom.TLSRequired {
		return endpointdom.TLSStatusRequired, nil
	}
	return endpointdom.TLSStatusPreferred, nil
}

// postgresSupportsTLS sends an SSLRequest and reads the single-byte reply.
// A truncated or closed connection is an error, never a TLS verdict — that
// conflation is what let unreachable endpoints report as healthy.
func postgresSupportsTLS(conn net.Conn) (bool, error) {
	// SSLRequest: int32 length = 8, int32 code = 80877103.
	msg := make([]byte, 8)
	binary.BigEndian.PutUint32(msg[0:4], 8)
	binary.BigEndian.PutUint32(msg[4:8], 80877103)
	if _, err := conn.Write(msg); err != nil {
		return false, fmt.Errorf("send postgres SSLRequest: %w", err)
	}

	buf := make([]byte, 1)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return false, fmt.Errorf("read postgres SSLRequest reply: %w", err)
	}
	switch buf[0] {
	case 'S':
		return true, nil
	case 'N':
		return false, nil
	case 'E':
		// The server rejected the startup attempt outright.
		return false, fmt.Errorf("postgres refused the connection during TLS negotiation")
	default:
		return false, fmt.Errorf("unexpected postgres SSLRequest reply %q", buf[0])
	}
}

// clientSSL is the CLIENT_SSL capability flag in the MySQL handshake.
const clientSSL = 0x0800

// mysqlSupportsTLS reads the server greeting, which is sent before any
// authentication, and tests the CLIENT_SSL capability bit.
func mysqlSupportsTLS(conn net.Conn) (bool, error) {
	// Packet header: 3-byte little-endian length + 1-byte sequence id.
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return false, fmt.Errorf("read mysql greeting header: %w", err)
	}
	length := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
	if length < 2 || length > 1<<20 {
		return false, fmt.Errorf("implausible mysql greeting length %d", length)
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(conn, body); err != nil {
		return false, fmt.Errorf("read mysql greeting: %w", err)
	}
	if body[0] == 0xFF {
		// ERR packet — e.g. the host is not allowed to connect.
		return false, fmt.Errorf("mysql refused the connection: %s", errPacketMessage(body))
	}

	// Body layout: protocol version (1) + NUL-terminated server version +
	// connection id (4) + auth-plugin-data-part-1 (8) + filler (1) +
	// capability_flags_lower (2).
	i := 1
	for i < len(body) && body[i] != 0 {
		i++
	}
	i++ // skip the NUL
	offset := i + 4 + 8 + 1
	if offset+2 > len(body) {
		return false, fmt.Errorf("mysql greeting truncated before capability flags")
	}
	capabilities := binary.LittleEndian.Uint16(body[offset : offset+2])
	return capabilities&clientSSL != 0, nil
}

// errPacketMessage extracts the human-readable part of a MySQL ERR packet.
func errPacketMessage(body []byte) string {
	// 0xFF + error code (2) + optional "#SQLSTATE" (6) + message.
	if len(body) < 3 {
		return "unknown error"
	}
	msg := body[3:]
	if len(msg) > 6 && msg[0] == '#' {
		msg = msg[6:]
	}
	return string(msg)
}
