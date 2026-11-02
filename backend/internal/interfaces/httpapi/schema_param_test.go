package httpapi

import (
	"strings"
	"testing"
)

func TestSchemaParam(t *testing.T) {
	got, err := schemaParam(" public, crm,,public ,billing")
	if err != nil || strings.Join(got, "|") != "public|crm|billing" {
		t.Errorf("schemaParam = %v, %v", got, err)
	}
	if got, err := schemaParam(""); got != nil || err != nil {
		t.Errorf("empty = %v, %v", got, err)
	}
	if _, err := schemaParam(strings.Repeat("x", 129)); err == nil {
		t.Error("long name accepted")
	}
	many := make([]string, maxSchemaFilter+1)
	for i := range many {
		many[i] = "s" + strings.Repeat("a", i%5) + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	if _, err := schemaParam(strings.Join(many, ",")); err == nil {
		t.Error("too many schemas accepted")
	}
}
