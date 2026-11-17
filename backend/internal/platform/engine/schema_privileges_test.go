package engine

import (
	"strings"
	"testing"
)

func TestCheckDefaultPrivileges(t *testing.T) {
	got, err := checkDefaultPrivileges("tables", []string{"insert", "select", "SELECT"})
	if err != nil || strings.Join(got, ",") != "SELECT,INSERT" {
		t.Errorf("tables: %v %v", got, err)
	}
	if got, _ := checkDefaultPrivileges("functions", []string{"all"}); strings.Join(got, ",") != "EXECUTE" {
		t.Errorf("ALL on functions = %v", got)
	}
	for _, c := range []struct{ kind, priv string }{{"tables", "EXECUTE"}, {"views", "SELECT"}, {"tables", "SELECT; DROP"}} {
		if _, err := checkDefaultPrivileges(c.kind, []string{c.priv}); err == nil {
			t.Errorf("%s on %s accepted", c.priv, c.kind)
		}
	}
	if _, err := checkDefaultPrivileges("tables", nil); err == nil {
		t.Error("no privileges accepted")
	}
}

func TestSearchPathSQL(t *testing.T) {
	got, err := SearchPathSQL([]string{"$user", "sales", `we"ird`})
	if err != nil || got != `SET search_path TO "$user", "sales", "we""ird"` {
		t.Errorf("got %q %v", got, err)
	}
	if _, err := SearchPathSQL(nil); err == nil {
		t.Error("empty path accepted")
	}
	if _, err := SearchPathSQL([]string{"a\nb"}); err == nil {
		t.Error("newline accepted")
	}
}
