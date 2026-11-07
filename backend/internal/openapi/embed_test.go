package openapi

import (
	"bytes"
	"testing"
)

func TestSpecForStampsInfoVersion(t *testing.T) {
	got := SpecFor("1.2.3")
	if !bytes.Contains(got, []byte("\ninfo:\n  title: Fleetdock API\n  version: 1.2.3\n")) {
		t.Fatalf("info.version not stamped:\n%s", got[:200])
	}
	if !bytes.Equal(SpecFor("dev"), Spec) {
		t.Fatal("a dev build must serve the spec unchanged")
	}
	if len(got)-len(Spec) > 16 || bytes.Count(got, []byte("version: 1.2.3")) != 1 {
		t.Fatal("only info.version may change")
	}
}
