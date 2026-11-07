package httpapi

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestOpenAPI_GenerateSchemas prints JSON Schema for documentedResponses when
// FLEETDOCK_GEN_SCHEMAS is set: a starting point for components.schemas
// after a response type changes (scripts/openapi-sync-schemas.py merges it).
func TestOpenAPI_GenerateSchemas(t *testing.T) {
	if os.Getenv("FLEETDOCK_GEN_SCHEMAS") == "" {
		t.Skip("set FLEETDOCK_GEN_SCHEMAS=1 to print schemas")
	}
	refs := map[reflect.Type]string{}
	for name, v := range documentedResponses {
		refs[reflect.TypeOf(v)] = name
	}
	out := map[string]any{}
	for name, v := range documentedResponses {
		out[name] = objectSchema(reflect.TypeOf(v), refs, true)
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	_ = os.WriteFile(os.Getenv("FLEETDOCK_GEN_SCHEMAS"), b, 0o644)
}

var timeType = reflect.TypeOf(time.Time{})

func objectSchema(t reflect.Type, refs map[reflect.Type]string, top bool) map[string]any {
	props := map[string]any{}
	var required []string
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				walk(f.Type)
				continue
			}
			if !f.IsExported() {
				continue
			}
			parts := strings.Split(f.Tag.Get("json"), ",")
			name := parts[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			omit := len(parts) > 1 && strings.Contains(f.Tag.Get("json"), "omitempty")
			props[name] = typeSchema(f.Type, refs)
			if !omit && f.Type.Kind() != reflect.Pointer {
				required = append(required, name)
			}
		}
	}
	walk(t)
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func typeSchema(t reflect.Type, refs map[reflect.Type]string) map[string]any {
	nullable := false
	if t.Kind() == reflect.Pointer {
		nullable = true
		t = t.Elem()
	}
	var s map[string]any
	switch {
	case refs[t] != "":
		s = map[string]any{"$ref": "#/components/schemas/" + refs[t]}
	case t == timeType:
		s = map[string]any{"type": "string", "format": "date-time"}
	case t.Name() == "RawMessage":
		s = map[string]any{}
	case t.Name() == "UUID":
		s = map[string]any{"type": "string", "format": "uuid"}
	default:
		switch t.Kind() {
		case reflect.String:
			s = map[string]any{"type": "string"}
		case reflect.Bool:
			s = map[string]any{"type": "boolean"}
		case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
			s = map[string]any{"type": "integer"}
		case reflect.Float32, reflect.Float64:
			s = map[string]any{"type": "number"}
		case reflect.Slice, reflect.Array:
			s = map[string]any{"type": "array", "items": typeSchema(t.Elem(), refs)}
		case reflect.Map:
			s = map[string]any{"type": "object", "additionalProperties": typeSchema(t.Elem(), refs)}
		case reflect.Struct:
			s = objectSchema(t, refs, false)
		case reflect.Interface:
			s = map[string]any{}
		default:
			s = map[string]any{}
		}
	}
	if nullable && s["type"] != nil {
		s["type"] = []any{s["type"], "null"}
	}
	return s
}
