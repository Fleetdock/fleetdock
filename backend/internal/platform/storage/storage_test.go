package storage

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/netsafe"
)

// Backup storage endpoints are user input and follow the database host
// rules. New refuses a forbidden endpoint before any request (minio-go would
// otherwise retry the refused dial until its context ends), and the
// transport refuses it again at connect time.
func TestEndpointsFollowTheDialPolicy(t *testing.T) {
	// Proxies would make the transport dial the proxy, not the endpoint.
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(k, "")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Cleanup(func() { netsafe.ConfigureDB(nil) })

	cfg := func(endpoint string) Config {
		return Config{Endpoint: endpoint, Bucket: "backups", AccessKey: "k", SecretKey: "s"}
	}

	netsafe.ConfigureDB(&netsafe.DBPolicy{AllowLoopback: false})
	for _, endpoint := range []string{"http://169.254.169.254", "https://169.254.169.254:9000", srv.URL} {
		if _, err := New(cfg(endpoint)); !errors.Is(err, netsafe.ErrDisallowedDBHost) {
			t.Errorf("New(%s): err = %v, want the host refused", endpoint, err)
		}
	}
	// The connect-time guard, which also covers a name that resolves
	// differently later.
	client := &http.Client{Transport: netsafe.ServiceTransport()}
	if resp, err := client.Get(srv.URL); err == nil || !errors.Is(err, netsafe.ErrDisallowedDBHost) {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Errorf("transport reached loopback in production: %v", err)
	}

	// Development allows loopback (MinIO on this machine).
	netsafe.ConfigureDB(&netsafe.DBPolicy{AllowLoopback: true})
	if _, err := New(cfg(srv.URL)); err != nil {
		t.Errorf("loopback allowed, New refused: %v", err)
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("loopback allowed, transport refused: %v", err)
	}
	_ = resp.Body.Close()

	// Without a policy (the agent) nothing is refused.
	netsafe.ConfigureDB(nil)
	if _, err := New(cfg("http://169.254.169.254")); err != nil {
		t.Errorf("no policy: %v", err)
	}
}
