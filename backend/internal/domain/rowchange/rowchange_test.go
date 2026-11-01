package rowchange

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

func sp(s string) *string { return &s }

func TestImageRoundTrip(t *testing.T) {
	in := Image{"text": sp("héllo"), "null": nil, "bin": sp("\x00\xff\x01"), "nul": sp("a\x00b"), "empty": sp("")}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"bin":{"b64":"AP8B"}`) || !strings.Contains(string(b), `"text":"héllo"`) {
		t.Fatalf("encoded = %s", b)
	}
	var out Image
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) || out["null"] != nil {
		t.Fatalf("decoded = %v", out)
	}
	for c, v := range in {
		if v != nil && *out[c] != *v {
			t.Errorf("%s = %q, want %q", c, *out[c], *v)
		}
	}
	var none Image
	if err := json.Unmarshal([]byte("null"), &none); err != nil || none != nil {
		t.Fatalf("null image = %v, %v", none, err)
	}
	if b, _ := json.Marshal(Image(nil)); string(b) != "null" {
		t.Fatalf("nil image = %s", b)
	}
}

func TestFromEngineTooLarge(t *testing.T) {
	big := strings.Repeat("x", MaxEntryBytes)
	c := FromEngine(uuid.New(), "t", SourceEditor, &engine.RowChange{
		Op: "delete", Key: engine.RowValues{"id": sp("1")}, Before: engine.RowValues{"id": sp("1"), "doc": &big}, Undoable: true,
	})
	if c.Undoable || c.Before != nil || c.Key == nil || c.Note == "" {
		t.Fatalf("change = %+v", c)
	}
	small := FromEngine(uuid.New(), "t", SourceEditor, &engine.RowChange{Op: "insert", Key: engine.RowValues{"id": sp("1")}, Undoable: true})
	if !small.Undoable || small.Columns == nil || small.Session == nil {
		t.Fatalf("small = %+v", small)
	}
}
