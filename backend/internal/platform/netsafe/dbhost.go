package netsafe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

// DBPolicy restricts which addresses the control plane may open database
// connections to. Instance hosts are user-supplied, so without it anyone with
// instance:write could point an "external instance" at loopback services,
// cloud metadata, or the control plane's own metadata database, and use the
// connection errors as a port scanner.
//
// Link-local and metadata addresses are always refused. Loopback is refused
// unless AllowLoopback is set (local development, where the database really is
// on localhost). Private RFC1918 ranges stay reachable: databases on a LAN are
// the normal case for a self-hosted control plane.
type DBPolicy struct {
	AllowLoopback bool
	// Deny lists host:port pairs that must never be dialed (the metadata
	// database). Hosts are resolved, so any name for the same IP matches.
	Deny []string
}

var (
	dbPolicyMu sync.RWMutex
	dbPolicy   *DBPolicy
)

// ConfigureDB installs the database dial policy. Until it is called (e.g. in
// the agent, which legitimately dials 127.0.0.1) no restriction applies.
func ConfigureDB(p *DBPolicy) {
	dbPolicyMu.Lock()
	dbPolicy = p
	dbPolicyMu.Unlock()
}

func currentDBPolicy() *DBPolicy {
	dbPolicyMu.RLock()
	defer dbPolicyMu.RUnlock()
	return dbPolicy
}

// ErrDisallowedDBHost is returned for addresses the policy refuses.
var ErrDisallowedDBHost = errors.New("this database host is not allowed")

// dbBlocked applies the policy to one resolved address.
func (p *DBPolicy) dbBlocked(ctx context.Context, ip net.IP, port int) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 169 && v4[1] == 254 {
		return true
	}
	if ip.Equal(net.ParseIP("fd00:ec2::254")) {
		return true
	}
	if ip.IsLoopback() && !p.AllowLoopback {
		return true
	}
	for _, d := range p.Deny {
		h, ps, err := net.SplitHostPort(d)
		if err != nil {
			continue
		}
		if dp, _ := strconv.Atoi(ps); dp != port {
			continue
		}
		ips, err := lookup(ctx, h)
		if err != nil {
			continue
		}
		for _, dip := range ips {
			if dip.Equal(ip) || (dip.IsLoopback() && ip.IsLoopback()) {
				return true
			}
		}
	}
	return false
}

func lookup(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

// CheckDBHost resolves host and reports ErrDisallowedDBHost if any of its
// addresses is refused by the configured policy. It is the early, friendly
// check run when an instance is saved; DialDB re-checks at connect time.
func CheckDBHost(ctx context.Context, host string, port int) error {
	p := currentDBPolicy()
	if p == nil {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := lookup(cctx, host)
	if err != nil {
		// Unresolvable now is not a policy violation; the connection test
		// will report it. DialDB still guards the real connection.
		return nil
	}
	for _, ip := range ips {
		if p.dbBlocked(cctx, ip, port) {
			return fmt.Errorf("%w: %s resolves to %s", ErrDisallowedDBHost, host, ip)
		}
	}
	return nil
}

// DialDB dials a database address, enforcing the policy against the IP that
// is actually connected to (so DNS rebinding cannot slip past CheckDBHost).
func DialDB(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	p := currentDBPolicy()
	if p == nil {
		return d.DialContext(ctx, network, addr)
	}
	host, ps, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(ps)
	ips, err := lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	lastErr := fmt.Errorf("%w: %s", ErrDisallowedDBHost, host)
	for _, ip := range ips {
		if p.dbBlocked(ctx, ip, port) {
			continue
		}
		conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), ps))
		if err != nil {
			lastErr = err
			continue
		}
		return conn, nil
	}
	return nil, lastErr
}
