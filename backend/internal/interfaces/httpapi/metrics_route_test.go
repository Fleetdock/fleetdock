package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authapp "github.com/Fleetdock/fleetdock/backend/internal/app/auth"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/auth"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/metrics"
)

func TestMetricsRoute(t *testing.T) {
	get := func(h http.Handler, auth string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}

	authn := NewAuthenticator(authapp.NewService(&stubAuthRepo{}, nil, auth.NewJWT("metrics-test", time.Hour)))
	off := NewRouter(RouterDeps{Authn: authn})
	if rec := get(off, "Bearer anything"); rec.Code != http.StatusNotFound {
		t.Errorf("without FLEETDOCK_METRICS_TOKEN: %d, want 404", rec.Code)
	}

	on := NewRouter(RouterDeps{Authn: authn, MetricsToken: "scrape-me", MetricsGauges: func(context.Context) []metrics.Gauge {
		return []metrics.Gauge{{Name: "databases", Help: "Databases.", Value: 7}}
	}})
	for _, bad := range []string{"", "Bearer wrong", "scrape-me", "Bearer scrape-me-too"} {
		if rec := get(on, bad); rec.Code != http.StatusUnauthorized {
			t.Errorf("auth %q: %d, want 401", bad, rec.Code)
		}
	}
	rec := get(on, "Bearer scrape-me")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "fleetdock_databases 7") ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Fatalf("metrics = %d %q\n%s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}
