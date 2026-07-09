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
