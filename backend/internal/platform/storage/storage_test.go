package storage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/netsafe"
)

func TestEndpointsFollowTheDialPolicy(t *testing.T) {
	// The policy applies to the address dialed; with a proxy configured that
	// is the proxy's (as for webhooks). Test the direct path.
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(k, "")
	}
	// A MinIO-like endpoint on loopback that would answer.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	netsafe.ConfigureDB(&netsafe.DBPolicy{AllowLoopback: false})
	t.Cleanup(func() { netsafe.ConfigureDB(nil) })

	for _, endpoint := range []string{"http://169.254.169.254", srv.URL} {
		c, err := New(Config{Endpoint: endpoint, Bucket: "backups", AccessKey: "k", SecretKey: "s"})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = c.TestAccess(ctx)
		cancel()
		if err == nil || (!errors.Is(err, netsafe.ErrDisallowedDBHost) && !strings.Contains(err.Error(), "not allowed")) {
			t.Errorf("%s: err = %v, want the host refused", endpoint, err)
		}
	}

	// Development (loopback allowed) reaches the local endpoint.
	netsafe.ConfigureDB(&netsafe.DBPolicy{AllowLoopback: true})
	c, err := New(Config{Endpoint: srv.URL, Bucket: "backups", AccessKey: "k", SecretKey: "s"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.TestAccess(ctx); err != nil && strings.Contains(err.Error(), "not allowed") {
		t.Errorf("loopback allowed, but refused: %v", err)
	}
}
