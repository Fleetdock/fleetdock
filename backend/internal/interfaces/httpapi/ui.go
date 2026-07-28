package httpapi

import (
	"net/http"
	"strings"
)

// apiOwnedPath reports whether the control plane owns a path. Everything else
// belongs to the bundled dashboard.
//
// Keep this in sync with the routes registered in NewRouter, and with apiPaths
// in frontend/next.config.mjs. A path that falls through here reaches the
// dashboard and gets an HTML 404 rather than a JSON error.
func apiOwnedPath(p string) bool {
	switch p {
	case "/healthz", "/readyz", "/openapi.yaml", "/docs", "/install.sh":
		return true
	}
	return p == "/v1" || strings.HasPrefix(p, "/v1/") ||
		strings.HasPrefix(p, "/agent/")
}

// splitUI dispatches API paths to api and everything else to ui.
//
// The split is a plain predicate rather than a "/" pattern on the API ServeMux
// because ServeMux only synthesises 405 Method Not Allowed when *no* pattern
// matched. A bare "/" matches every path and every method, so registering one
// would silently turn `POST /healthz` from a 405 into an HTML 404 from the
// dashboard. Staying outside the mux also keeps its path cleaning and redirects
// off dashboard URLs.
func splitUI(api, ui http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiOwnedPath(r.URL.Path) {
			api.ServeHTTP(w, r)
			return
		}
		ui.ServeHTTP(w, r)
	})
}
