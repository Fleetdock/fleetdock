package uiproxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// Handler serves the dashboard: 503 until the child process is accepting
// connections, then a reverse proxy to it.
func (s *Supervisor) Handler() http.Handler { return s.handler }

func (s *Supervisor) newHandler() http.Handler {
	target := &url.URL{Scheme: "http", Host: s.Addr()}

	rp := &httputil.ReverseProxy{
		// Rewrite rather than Director: Director would append to a
		// client-supplied X-Forwarded-For, letting a caller prepend whatever
		// address it likes. SetXForwarded replaces the header outright.
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()
			// Next builds absolute URLs from the Host it sees, so it must be
			// the browser-facing one, not the loopback upstream.
			pr.Out.Host = pr.In.Host
		},

		// Headers are set on the upstream response, never on the outer
		// ResponseWriter: ReverseProxy copies upstream headers with Add, not
		// Set, so pre-setting Cache-Control here would leave two values on
		// /_next/static/* — Next's `immutable` plus ours — and the more
		// restrictive one would win, defeating asset caching entirely.
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Set("X-Content-Type-Options", "nosniff")
			resp.Header.Set("Referrer-Policy", "no-referrer")
			if resp.Header.Get("X-Frame-Options") == "" {
				resp.Header.Set("X-Frame-Options", "DENY")
			}
			resp.Header.Del("X-Powered-By")
			return nil
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Warn("dashboard proxy error", "path", r.URL.Path, "error", err.Error())
			http.Error(w, "dashboard unavailable", http.StatusBadGateway)
		},

		Transport: &http.Transport{
			MaxIdleConnsPerHost: 64,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.ready.Load() {
			w.Header().Set("Retry-After", "5")
			http.Error(w, "dashboard starting", http.StatusServiceUnavailable)
			return
		}
		rp.ServeHTTP(w, r)
	})
}
