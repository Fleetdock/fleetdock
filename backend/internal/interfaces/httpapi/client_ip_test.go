package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name  string
		xff   []string
		trust bool
		want  string
	}{
		{"untrusted header is ignored", []string{"1.2.3.4"}, false, "10.0.0.9"},
		{"no header", nil, true, "10.0.0.9"},
		{"replacing proxy (Caddy)", []string{"203.0.113.7"}, true, "203.0.113.7"},
		// nginx $proxy_add_x_forwarded_for keeps what the client sent and
		// appends the peer it saw: the client cannot choose the last entry.
		{"appending proxy with a spoofed entry", []string{"1.2.3.4, 203.0.113.7"}, true, "203.0.113.7"},
		{"several header lines", []string{"1.2.3.4", "203.0.113.7"}, true, "203.0.113.7"},
		{"blank last entry", []string{"203.0.113.7, "}, true, "10.0.0.9"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "10.0.0.9:51234"
		for _, v := range c.xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		if got := clientIP(r, c.trust); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
