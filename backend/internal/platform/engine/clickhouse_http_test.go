package engine

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// chTestParams points a ClickHouse client at a local test server.
func chTestParams(t *testing.T, srv *httptest.Server) ConnParams {
	t.Helper()
	host, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	n, _ := strconv.Atoi(port)
	return ConnParams{Host: host, Port: n, User: "default", Password: "s3cret-pw", TLSMode: TLSDisable}
}

func TestClickHouseDoesNotFollowRedirects(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		var reached atomic.Bool
		target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Store(true) }))
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+"/internal", status)
		}))
		_, err := (&ClickHouse{}).Ping(context.Background(), chTestParams(t, origin))
		origin.Close()
		target.Close()
		if err == nil {
			t.Errorf("HTTP %d: redirect was accepted", status)
		}
		if reached.Load() {
			t.Errorf("HTTP %d: redirect was followed, carrying the credentials to another host", status)
		}
	}
}

func TestClickHouseDoesNotEchoOtherServices(t *testing.T) {
	const secret = "internal-admin-panel-token"
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError, http.StatusNotFound} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			// Even text shaped like a ClickHouse exception must not be shown.
			_, _ = w.Write([]byte("Code: 1. DB::Exception: " + secret + "\n<html>" + secret + "</html>"))
		}))
		_, err := (&ClickHouse{}).Ping(context.Background(), chTestParams(t, srv))
		srv.Close()
		if err == nil {
			t.Fatalf("HTTP %d from a non-ClickHouse server was accepted", status)
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("HTTP %d: error echoes the response body: %v", status, err)
		}
		if !strings.Contains(err.Error(), "not a ClickHouse server") {
			t.Errorf("HTTP %d: error = %v, want a not-ClickHouse error", status, err)
		}
	}
}

func TestClickHouseReportsServerExceptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-ClickHouse-Exception-Code", "516")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("Code: 516. DB::Exception: default: Authentication failed. (AUTHENTICATION_FAILED) (version 25.8.1.1 (official build))\n"))
	}))
	defer srv.Close()
	_, err := (&ClickHouse{}).Ping(context.Background(), chTestParams(t, srv))
	var ce *ClickHouseError
	if !errors.As(err, &ce) || ce.Code != chAuthenticationFailed || !IsLoginRejected(err) {
		t.Fatalf("err = %v, want AUTHENTICATION_FAILED", err)
	}
}

func TestClickHouseAcceptsClickHouseResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-ClickHouse-Query-Id", "q1")
		w.Header().Set("X-ClickHouse-Format", chResultFormat)
		_, _ = w.Write([]byte("[\"version()\"]\n[\"String\"]\n[\"25.8.1.1\"]\n"))
	}))
	defer srv.Close()
	v, err := (&ClickHouse{}).Ping(context.Background(), chTestParams(t, srv))
	if err != nil || v != "25.8.1.1" {
		t.Fatalf("Ping = %q, %v; want 25.8.1.1", v, err)
	}
}
