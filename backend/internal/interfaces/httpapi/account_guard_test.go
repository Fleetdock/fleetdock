package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAccountGuard(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	g := newAccountGuard(3, 15*time.Minute)
	g.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		g.Failed("Admin@Example.com ")
		now = now.Add(time.Minute)
	}
	if locked, _ := g.Locked("admin@example.com"); locked {
		t.Fatal("locked after 2 of 3 failures")
	}
	g.Failed("admin@example.com")
	locked, wait := g.Locked("ADMIN@example.com")
	if !locked || wait != 13*time.Minute {
		t.Fatalf("after 3 failures: locked=%v wait=%v, want 13m (the first failure ages out)", locked, wait)
	}
	if locked, _ := g.Locked("other@example.com"); locked {
		t.Fatal("another account is locked")
	}
	now = now.Add(13*time.Minute + time.Second)
	if locked, _ := g.Locked("admin@example.com"); locked {
		t.Fatal("still locked after the window")
	}
	g.Failed("admin@example.com")
	g.Succeeded("admin@example.com")
	if len(g.fails) != 0 {
		t.Fatalf("a successful sign-in must clear failures: %v", g.fails)
	}
}

// After 10 wrong passwords the account answers 429 even to the right one,
// and does not check it.
func TestLoginLocksAccount(t *testing.T) {
	h := newSmokeHandler(t)
	login := func(pw string) int {
		body, _ := jsonBody(map[string]string{"email": "admin@example.com", "password": pw})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/auth/login", body))
		return rec.Code
	}
	for i := 0; i < 10; i++ {
		if code := login("wrong-password"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d, want 401", i+1, code)
		}
	}
	if code := login("secret123"); code != http.StatusTooManyRequests {
		t.Fatalf("the right password after 10 failures: %d, want 429", code)
	}
}

func jsonBody(v any) (*bytes.Reader, error) {
	b, err := json.Marshal(v)
	return bytes.NewReader(b), err
}
