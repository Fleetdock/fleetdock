package main

import (
	"net"
	"net/url"
	"os/exec"
	"strings"
)

// primaryAddress returns the IPv4 this host uses to reach the control plane,
// or the first non-loopback IPv4 when route detection is unavailable.
func primaryAddress(controlPlaneURL string) string {
	if host := controlPlaneHost(controlPlaneURL); host != "" {
		if ip := routeSrcIP(host); ip != "" {
			return ip
		}
	}
	return firstNonLoopbackIPv4()
}

func controlPlaneHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		return u.Host
	}
	return host
}

func routeSrcIP(dest string) string {
	if _, err := exec.LookPath("ip"); err != nil {
		return ""
	}
	out, err := exec.Command("ip", "-4", "route", "get", dest).Output()
	if err != nil {
		return ""
	}
	return parseRouteSrc(string(out))
}

// parseRouteSrc extracts the source IPv4 from `ip route get` output.
func parseRouteSrc(output string) string {
	fields := strings.Fields(output)
	for i, f := range fields {
		if f == "src" && i+1 < len(fields) && isIPv4(fields[i+1]) {
			return fields[i+1]
		}
	}
	return ""
}

func firstNonLoopbackIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if v4 := ipNet.IP.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}

func isIPv4(s string) bool {
	return net.ParseIP(s).To4() != nil
}
