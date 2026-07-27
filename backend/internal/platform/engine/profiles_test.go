package engine

import "testing"

func TestApplyProfileMapping(t *testing.T) {
	if ProfileReadonly == "" || ProfileReadWrite == "" || ProfileAdmin == "" {
		t.Fatal("profiles defined")
	}
}
