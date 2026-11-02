package httpapi

import (
	"net/http"
	"sync"
	"time"
)

// loginLimiter is a small fixed-window rate limiter keyed by client IP,
// protecting the password login endpoint from brute force. In-memory and
// per-instance by design — good enough for a single control plane node.
type loginLimiter struct {
	mu         sync.Mutex
	hits       map[string][]time.Time
	limit      int
	window     time.Duration
	trustProxy bool
	message    string // the 429's message
}

func newLoginLimiter(limit int, window time.Duration, trustProxy bool) *loginLimiter {
	l := &loginLimiter{hits: make(map[string][]time.Time), limit: limit, window: window, trustProxy: trustProxy,
		message: "too many login attempts; try again in a minute"}
	go l.gc()
	return l
}

// Allow records an attempt for key and reports whether it is within limits.
func (l *loginLimiter) Allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.limit {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	return true
}

func (l *loginLimiter) gc() {
	for range time.Tick(5 * time.Minute) {
		cutoff := time.Now().Add(-l.window)
		l.mu.Lock()
		for k, ts := range l.hits {
			if len(ts) == 0 || !ts[len(ts)-1].After(cutoff) {
				delete(l.hits, k)
			}
		}
		l.mu.Unlock()
	}
}

// Middleware applies the limiter to the wrapped handler.
func (l *loginLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(l.clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, errorBody{Error: errorDetail{
				Code:    "rate_limited",
				Message: l.message,
			}})
			return
		}
		next(w, r)
	}
}

// clientIP resolves the caller's IP for rate-limiting (see the package-level
// clientIP): behind a trusted proxy, every caller does not collapse into one
// shared bucket.
func (l *loginLimiter) clientIP(r *http.Request) string {
	return clientIP(r, l.trustProxy)
}

// securityHeaders sets conservative defaults appropriate for a JSON API.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		// API responses are JSON/CSV and never need to load anything.
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
