package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newSplitHandler mirrors the production chain closely enough to exercise the
// split: an API ServeMux behind securityHeaders, and a stub dashboard standing
// in for the reverse proxy.
func newSplitHandler(ui http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /v1/overview", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})
	return splitUI(securityHeaders(mux), ui)
}

// stubUI stands in for the proxied dashboard, including the immutable caching
// Next.js applies to hashed assets.
func stubUI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_next/static/chunks/abc123.js" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte("//js"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html></html>"))
	})
}

// A "/" pattern on the API mux would match every method and suppress the mux's
// own 405 synthesis, silently turning wrong-method API calls into dashboard
// 404s. The split must keep method mismatches inside the API branch.
func TestSplitUI_PreservesMethodNotAllowed(t *testing.T) {
	h := newSplitHandler(stubUI())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/healthz", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /healthz: expected 405, got %d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("POST /healthz: expected Allow: GET, HEAD, got %q", got)
	}
}

// The dashboard must not inherit the API's Cache-Control: no-store, and must
// not end up with two Cache-Control values either — the more restrictive one
// would win and no asset would ever be cached.
func TestSplitUI_DoesNotOverrideAssetCaching(t *testing.T) {
	h := newSplitHandler(stubUI())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_next/static/chunks/abc123.js", nil))

	values := rec.Header().Values("Cache-Control")
	if len(values) != 1 {
		t.Fatalf("expected exactly one Cache-Control header, got %d: %v", len(values), values)
	}
	if values[0] != "public, max-age=31536000, immutable" {
		t.Fatalf("dashboard caching was overridden: %q", values[0])
	}
}

// API responses keep no-store: they carry tokens, credentials and connection
// strings that must never be held by an intermediary.
func TestSplitUI_APIKeepsNoStore(t *testing.T) {
	h := newSplitHandler(stubUI())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	values := rec.Header().Values("Cache-Control")
	if len(values) != 1 || values[0] != "no-store" {
		t.Fatalf("expected exactly one no-store on the API, got %v", values)
	}
}

func TestApiOwnedPath(t *testing.T) {
	api := []string{
		"/healthz", "/readyz", "/openapi.yaml", "/docs", "/install.sh",
		"/v1", "/v1/auth/login", "/v1/servers/abc",
		"/agent/v1/register", "/agent/v1/binary/linux/amd64",
	}
	for _, p := range api {
		if !apiOwnedPath(p) {
			t.Errorf("expected %q to be API-owned", p)
		}
	}

	ui := []string{
		"/", "/login", "/dashboard", "/servers", "/servers/abc",
		"/_next/static/chunks/abc.js", "/favicon.ico", "/docsomething",
		"/v1foo", "/manifest.webmanifest",
	}
	for _, p := range ui {
		if apiOwnedPath(p) {
			t.Errorf("expected %q to route to the dashboard", p)
		}
	}
}

// Without a bundled dashboard — a bare `api` binary, or a dev run — non-API
// paths must stay inside the API chain and produce a JSON error, exactly as
// they did before the dashboard was merged in. They must never fall through to
// a UI that is not there.
func TestNilUI_NonAPIPathStaysInTheAPI(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// Mirrors NewRouter's nil-UI branch.
	h := securityHeaders(mux)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 from the mux without a dashboard, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
		t.Fatalf("non-API path produced HTML without a bundled dashboard: %q", ct)
	}
}
