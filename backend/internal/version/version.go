// Package version reports which Fleetdock build is running and decides
// whether an agent is older than the control plane.
//
// Release builds set Version, Commit and Date with the linker:
//
//	go build -ldflags "-X github.com/Fleetdock/fleetdock/backend/internal/version.Version=1.0.0 ..."
//
// A plain `go build` reports "dev" and the VCS revision Go recorded, if any.
package version

import (
	"runtime/debug"
	"strconv"
	"strings"
)

// Set by -ldflags -X at release time.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// MinAgentVersion is the oldest agent this control plane still drives. Older
// agents are shown as unsupported. Every agent released so far speaks the
// current /agent/v1 protocol, so this stays at the first release until the
// protocol changes.
const MinAgentVersion = "0.1.0"

// Info is what /v1/version and `api version` report.
type Info struct {
	Version         string `json:"version"`
	Commit          string `json:"commit,omitempty"`
	Date            string `json:"date,omitempty"`
	MinAgentVersion string `json:"min_agent_version"`
}

// Get returns the running build's version information.
func Get() Info {
	commit, date := Commit, Date
	if commit == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					commit = s.Value
				case "vcs.time":
					if date == "" {
						date = s.Value
					}
				}
			}
		}
	}
	if len(commit) > 12 {
		commit = commit[:12]
	}
	return Info{Version: Normalize(Version), Commit: commit, Date: date, MinAgentVersion: MinAgentVersion}
}

// Normalize drops a leading "v", so "v1.2.3" and "1.2.3" read the same.
func Normalize(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 1 && (v[0] == 'v' || v[0] == 'V') && v[1] >= '0' && v[1] <= '9' {
		return v[1:]
	}
	return v
}

// Parse reads "1.2.3" or "1.2.3-rc.1" (with or without a leading v) into its
// numeric parts and pre-release suffix. ok is false for anything else,
// including "dev".
func Parse(v string) (parts [3]int, pre string, ok bool) {
	v = Normalize(v)
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v, pre = v[:i], v[i+1:]
	}
	fields := strings.Split(v, ".")
	if len(fields) != 3 {
		return parts, "", false
	}
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 || f == "" {
			return parts, "", false
		}
		parts[i] = n
	}
	return parts, pre, true
}

// Compare orders two versions as SemVer does (ignoring build metadata and
// comparing pre-release suffixes as plain strings, which is right for
// "rc.1" < "rc.2"). ok is false when either side does not parse.
func Compare(a, b string) (cmp int, ok bool) {
	pa, prea, oka := Parse(a)
	pb, preb, okb := Parse(b)
	if !oka || !okb {
		return 0, false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1, true
			}
			return 1, true
		}
	}
	switch {
	case prea == preb:
		return 0, true
	case prea == "": // a release sorts after its pre-releases
		return 1, true
	case preb == "":
		return -1, true
	case prea < preb:
		return -1, true
	default:
		return 1, true
	}
}

// AgentStatus says how an agent's reported version relates to this control
// plane: "current", "outdated" (older, still supported: an update is
// available), "unsupported" (older than MinAgentVersion) or "unknown" (no
// version, or a development build on either side).
func AgentStatus(agentVersion string) string {
	if strings.TrimSpace(agentVersion) == "" {
		return "unknown"
	}
	if c, ok := Compare(agentVersion, MinAgentVersion); ok && c < 0 {
		return "unsupported"
	}
	c, ok := Compare(agentVersion, Version)
	if !ok {
		return "unknown"
	}
	if c < 0 {
		return "outdated"
	}
	return "current"
}
