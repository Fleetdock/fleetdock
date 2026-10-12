package httpapi

import (
	"net/http"
	"strings"
	"time"
)

// The dashboard authenticates with an httpOnly session cookie instead of a
// JWT kept in localStorage, so injected script cannot read the session.
// Bearer tokens (Authorization header) remain the way API clients and
// automation authenticate.
const (
	sessionCookieName = "fleetdock_session"

	// csrfHeader must accompany every state-changing request authenticated
	// by the session cookie. Browsers only send a custom header cross-origin
	// after a CORS preflight the API does not grant, so a forged form post or
	// cross-site fetch cannot carry it. SameSite=Strict is a second layer.
	csrfHeader = "X-Fleetdock-CSRF"
)

// SessionCookieConfig controls how the session cookie is issued.
type SessionCookieConfig struct {
	// TTL is the session lifetime; it should match the JWT TTL.
	TTL time.Duration
	// Secure forces the Secure attribute. When false, the cookie is still
	// marked Secure for requests that arrived over TLS or via an HTTPS proxy.
	Secure bool
}

func (c SessionCookieConfig) secure(r *http.Request) bool {
	return c.Secure || r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (c SessionCookieConfig) set(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(c.TTL.Seconds()),
		HttpOnly: true,
		Secure:   c.secure(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func (c SessionCookieConfig) clear(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.secure(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func sessionCookie(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// safeMethod reports whether the method cannot change state, so a cookie
// request with it needs no CSRF header.
func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}
