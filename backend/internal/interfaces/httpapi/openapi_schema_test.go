package httpapi

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// documentedResponses pairs each OpenAPI response schema with the Go type
// the handlers encode, so the documented fields cannot drift from the JSON.
var documentedResponses = map[string]any{
	"Server":        serverResponse{},
	"Instance":      instanceResponse{},
	"InstanceRef":   instanceRefResponse{},
	"SSHTunnel":     sshTunnelResponse{},
	"Database":      databaseResponse{},
	"Backup":        backupResponse{},
	"Operation":     operationResponse{},
	"LoginResponse": loginResponse{},
	"Me":            meResponse{},
	"User":          userResponse{},
	"Role":          roleResponse{},
	"RoleGrant":     roleGrantResponse{},
	"APIToken":      tokenResponse{},
	"NewAPIToken":   createTokenResponse{},
	"AgentToken":    regTokenResponse{},
	"NewAgentToken": createdRegTokenResponse{},
	"Destination":   destinationResponse{},
	"Schedule":      scheduleResponse{},
	"Channel":       channelResponse{},
	"AlertRule":     ruleResponse{},
}

func jsonFields(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			out = append(out, jsonFields(f.Type)...)
			continue
		}
		tag := f.Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "-" || !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out = append(out, name)
	}
	return out
}

func TestOpenAPI_SchemasMatchResponses(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	spec, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "openapi", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `yaml:"properties"`
				AllOf      []struct {
					Properties map[string]any `yaml:"properties"`
				} `yaml:"allOf"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(documentedResponses))
	for n := range documentedResponses {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		schema, ok := doc.Components.Schemas[name]
		if !ok {
			t.Errorf("components.schemas.%s is missing", name)
			continue
		}
		props := map[string]bool{}
		for p := range schema.Properties {
			props[p] = true
		}
		for _, part := range schema.AllOf {
			for p := range part.Properties {
				props[p] = true
			}
		}
		fields := map[string]bool{}
		for _, f := range jsonFields(reflect.TypeOf(documentedResponses[name])) {
			fields[f] = true
			if !props[f] {
				t.Errorf("%s: field %q is returned but not documented", name, f)
			}
		}
		for p := range props {
			if !fields[p] {
				t.Errorf("%s: %q is documented but not returned", name, p)
			}
		}
	}
}
