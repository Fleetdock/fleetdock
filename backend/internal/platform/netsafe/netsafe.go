// Package netsafe provides SSRF-resistant outbound HTTP.
//
// Notification webhooks (and any other user-supplied URL the control plane
// fetches) can otherwise be pointed at loopback or cloud-metadata endpoints
// (e.g. http://169.254.169.254/) to read credentials or reach internal
// services. GuardedClient enforces the check at dial time against the actual
// resolved IP, so DNS rebinding and redirect-based bypasses are also blocked.
//
// By design only loopback, link-local, unspecified and metadata addresses are
// blocked — RFC1918 private ranges remain reachable, because self-hosted
// webhook and alerting targets on a LAN are a legitimate use case.
package netsafe

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// blocked reports whether an IP must never be dialed for user-supplied URLs.
func blocked(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// IPv4/IPv6 cloud instance-metadata endpoints (link-local already covers
	// 169.254.169.254, but block explicitly for clarity and the IPv6 form).
	if v4 := ip.To4(); v4 != nil && v4[0] == 169 && v4[1] == 254 {
		return true
	}
	if ip.Equal(net.ParseIP("fd00:ec2::254")) {
		return true
	}
	return false
}

// guardedDialContext wraps a base dialer, rejecting any address whose resolved
// IP is blocked.
func guardedDialContext(base *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range ips {
			if blocked(ip) {
				lastErr = fmt.Errorf("netsafe: refusing to connect to disallowed address %s", ip)
				continue
			}
			conn, err := base.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err != nil {
				lastErr = err
				continue
			}
			return conn, nil
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("netsafe: no dialable address for %q", host)
		}
		return nil, lastErr
	}
}

// GuardedClient returns an http.Client that refuses to connect to loopback,
// link-local, unspecified or metadata addresses, with the given total timeout.
func GuardedClient(timeout time.Duration) *http.Client {
	base := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           guardedDialContext(base),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}
