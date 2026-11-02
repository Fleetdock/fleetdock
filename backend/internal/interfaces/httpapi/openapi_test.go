package httpapi

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPI_CoversRouterRoutes(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	routerPath := filepath.Join(repoRoot, "backend", "internal", "interfaces", "httpapi", "router.go")
	openAPIPath := filepath.Join(repoRoot, "backend", "internal", "openapi", "openapi.yaml")

	routerBytes, err := os.ReadFile(routerPath)
	if err != nil {
		t.Fatalf("read router: %v", err)
	}
	specBytes, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}

	routerRoutes := parseRouterRoutes(string(routerBytes))
	docRoutes := parseOpenAPIRoutes(specBytes)

	for route := range routerRoutes {
		if _, ok := docRoutes[route]; !ok {
			t.Errorf("router route %q missing from docs/openapi.yaml", route)
		}
	}
	for route := range docRoutes {
		if _, ok := routerRoutes[route]; !ok {
			t.Errorf("docs/openapi.yaml documents %q but router.go has no matching route", route)
		}
	}
}

var handleFuncRE = regexp.MustCompile(`HandleFunc\("([A-Z]+) ([^"]+)"`)

func parseRouterRoutes(src string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, m := range handleFuncRE.FindAllStringSubmatch(src, -1) {
		out[normalizeRoute(m[1], m[2])] = struct{}{}
	}
	return out
}

func parseOpenAPIRoutes(spec []byte) map[string]struct{} {
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		panic(err)
	}
	out := map[string]struct{}{}
	for path, methods := range doc.Paths {
		for method := range methods {
			if method == "parameters" {
				continue
			}
			out[normalizeRoute(strings.ToUpper(method), path)] = struct{}{}
		}
	}
	return out
}

func normalizeRoute(method, path string) string {
	return method + " " + path
}

// TestOpenAPI_OperationsAreComplete holds every operation to the 1.0 API
// contract: a summary, at least one success response, the shared error
// response, and an authentication scheme on every /v1 route except the
// public ones.
func TestOpenAPI_OperationsAreComplete(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	spec, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "openapi", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	type operation struct {
		Summary   string           `yaml:"summary"`
		Security  []map[string]any `yaml:"security"`
		Responses map[string]any   `yaml:"responses"`
	}
	var doc struct {
		Paths map[string]map[string]operation `yaml:"paths"`
	}
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	public := map[string]bool{"POST /v1/auth/login": true}
	for path, ops := range doc.Paths {
		for method, op := range ops {
			if method == "parameters" {
				continue
			}
			name := strings.ToUpper(method) + " " + path
			if op.Summary == "" {
				t.Errorf("%s: no summary", name)
			}
			success := false
			for code := range op.Responses {
				if strings.HasPrefix(code, "2") || strings.HasPrefix(code, "3") {
					success = true
				}
			}
			if !success {
				t.Errorf("%s: no success response", name)
			}
			if _, ok := op.Responses["default"]; !ok {
				t.Errorf("%s: no default (error) response", name)
			}
			if strings.HasPrefix(path, "/v1/") && !public[name] {
				schemes := map[string]bool{}
				for _, s := range op.Security {
					for k := range s {
						schemes[k] = true
					}
				}
				if !schemes["bearerAuth"] || !schemes["cookieAuth"] {
					t.Errorf("%s: security must list bearerAuth and cookieAuth, has %v", name, op.Security)
				}
			}
		}
	}
}

// Every tag an operation uses must be declared, or documentation generators
// drop those operations.
func TestOpenAPI_TagsDeclared(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	spec, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "openapi", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tags  []struct{ Name string } `yaml:"tags"`
		Paths map[string]map[string]struct {
			Tags []string `yaml:"tags"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, tag := range doc.Tags {
		declared[tag.Name] = true
	}
	for path, ops := range doc.Paths {
		for method, op := range ops {
			for _, tag := range op.Tags {
				if !declared[tag] {
					t.Errorf("%s %s uses undeclared tag %q", strings.ToUpper(method), path, tag)
				}
			}
		}
	}
}
