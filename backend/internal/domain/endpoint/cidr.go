package endpoint

import (
	"net"
	"strings"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// AllowAnywhere is the explicit CIDR representing unrestricted public access.
const AllowAnywhere = "0.0.0.0/0"

// NormalizeCIDRs validates and deduplicates CIDR strings.
func NormalizeCIDRs(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, apperr.Invalid("allowed_cidrs", "at least one allowed network is required")
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		cidr, err := ValidateCIDR(raw)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[cidr]; ok {
			continue
		}
		seen[cidr] = struct{}{}
		out = append(out, cidr)
	}
	return out, nil
}

// ValidateCIDR normalizes one IPv4 or IPv6 CIDR. A bare address is accepted and
// treated as a single host (/32 or /128), which is what people usually mean
// when they paste the output of "what is my IP".
func ValidateCIDR(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", apperr.Invalid("allowed_cidrs", "CIDR cannot be empty")
	}
	if raw == AllowAnywhere || raw == "::/0" {
		return AllowAnywhere, nil
	}

	if !strings.Contains(raw, "/") {
		ip := net.ParseIP(raw)
		if ip == nil {
			return "", apperr.Invalid("allowed_cidrs", "invalid IP address or CIDR: "+raw)
		}
		if v4 := ip.To4(); v4 != nil {
			return v4.String() + "/32", nil
		}
		return ip.String() + "/128", nil
	}

	ip, ipNet, err := net.ParseCIDR(raw)
	if err != nil {
		return "", apperr.Invalid("allowed_cidrs", "invalid CIDR: "+raw)
	}
	ones, bits := ipNet.Mask.Size()
	if ones == 0 && (bits == 32 || bits == 128) {
		return AllowAnywhere, nil
	}
	// Reject rather than silently widen. "10.1.2.3/24" almost always means the
	// author wanted that one host; masking it to 10.1.2.0/24 quietly grants
	// access to 255 addresses they never named.
	if !ip.Equal(ipNet.IP) {
		return "", apperr.Invalid("allowed_cidrs",
			"CIDR "+raw+" has host bits set; use "+ipNet.String()+" for the whole network, or the address alone for a single host")
	}
	return ipNet.String(), nil
}

// AllowsAnywhere reports whether the allowlist permits all addresses.
func AllowsAnywhere(cidrs []string) bool {
	for _, c := range cidrs {
		if c == AllowAnywhere || c == "::/0" {
			return true
		}
	}
	return false
}
