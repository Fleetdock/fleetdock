package version

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"1.0.0", "1.0.0", 0, true},
		{"v1.0.0", "1.0.0", 0, true},
		{"0.9.0", "1.0.0", -1, true},
		{"1.10.0", "1.9.3", 1, true},
		{"1.0.0-rc.1", "1.0.0", -1, true},
		{"1.0.0", "1.0.0-rc.2", 1, true},
		{"1.0.0-rc.1", "1.0.0-rc.2", -1, true},
		{"1.0.0+abc", "1.0.0", 0, true},
		{"dev", "1.0.0", 0, false},
		{"1.0", "1.0.0", 0, false},
		{"", "1.0.0", 0, false},
	}
	for _, c := range cases {
		got, ok := Compare(c.a, c.b)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Compare(%q, %q) = %d, %v; want %d, %v", c.a, c.b, got, ok, c.want, c.ok)
		}
	}
}

func TestAgentStatus(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })

	Version = "v1.2.0"
	for agent, want := range map[string]string{
		"1.2.0":  "current",
		"1.3.0":  "current",
		"1.1.9":  "outdated",
		"0.1.0":  "outdated",
		"0.0.9":  "unsupported",
		"":       "unknown",
		"custom": "unknown",
	} {
		if got := AgentStatus(agent); got != want {
			t.Errorf("AgentStatus(%q) with control plane 1.2.0 = %q, want %q", agent, got, want)
		}
	}

	Version = "dev"
	if got := AgentStatus("1.0.0"); got != "unknown" {
		t.Errorf("a development control plane cannot judge agents, got %q", got)
	}
}

func TestGetNormalizes(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "v1.0.0"
	if got := Get(); got.Version != "1.0.0" || got.MinAgentVersion != MinAgentVersion {
		t.Fatalf("Get() = %+v", got)
	}
}
