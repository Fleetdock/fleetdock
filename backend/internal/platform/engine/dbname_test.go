package engine

import (
	"strings"
	"testing"
)

func TestValidDBName(t *testing.T) {
	valid := []string{
		"test", "acceptix_prod", "a$b", "x",
		"tenant_ffc07774-1db5-47fc-8bf5-ef388bcba207",
		"app-db", "9lives", strings.Repeat("a", 64),
	}
	for _, name := range valid {
		if !validDBName(name) {
			t.Errorf("validDBName(%q) = false, want true", name)
		}
	}
	invalid := []string{
		"", "-leading", "--result-file=/tmp/x", "a b", "a`b", "a'b", `a"b`,
		"a;b", "a/b", "a.b", "a\\b", strings.Repeat("a", 65),
	}
	for _, name := range invalid {
		if validDBName(name) {
			t.Errorf("validDBName(%q) = true, want false", name)
		}
	}
}

// Charsets, collations and roles keep the stricter identRe.
func TestIdentReStillRejectsHyphen(t *testing.T) {
	if identRe.MatchString("utf8mb4-x") || validPGRole("role-x") {
		t.Fatal("identRe must keep rejecting '-'")
	}
}
