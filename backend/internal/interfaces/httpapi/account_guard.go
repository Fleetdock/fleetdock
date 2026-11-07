package httpapi

import (
	"strings"
	"sync"
	"time"
)

// accountGuard stops password guessing against one account from many
// addresses, which the per-address login limit cannot see: after limit
// failed sign-ins within window, the account refuses sign-in (without
// checking the password) until the oldest of those failures is window old.
// It is in memory, per process, like the address limit; a successful
// sign-in clears it.
type accountGuard struct {
	mu     sync.Mutex
	fails  map[string][]time.Time
	limit  int
	window time.Duration
	now    func() time.Time
}

func newAccountGuard(limit int, window time.Duration) *accountGuard {
	return &accountGuard{fails: map[string][]time.Time{}, limit: limit, window: window, now: time.Now}
}

func accountKey(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// recent drops failures older than the window; the caller holds mu.
func (g *accountGuard) recent(key string) []time.Time {
	cutoff := g.now().Add(-g.window)
	kept := g.fails[key][:0]
	for _, t := range g.fails[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(g.fails, key)
		return nil
	}
	g.fails[key] = kept
	return kept
}

// Locked reports whether the account is locked, and for how long.
func (g *accountGuard) Locked(email string) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	f := g.recent(accountKey(email))
	if len(f) < g.limit {
		return false, 0
	}
	return true, f[len(f)-g.limit].Add(g.window).Sub(g.now())
}

// Failed records a failed sign-in.
func (g *accountGuard) Failed(email string) {
	key := accountKey(email)
	if key == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	f := append(g.recent(key), g.now())
	if len(f) > g.limit {
		f = f[len(f)-g.limit:]
	}
	g.fails[key] = f
	if len(g.fails) > 100_000 { // bound memory under a spray of random emails
		for k := range g.fails {
			delete(g.fails, k)
			break
		}
	}
}

// Succeeded clears the account's failures.
func (g *accountGuard) Succeeded(email string) {
	g.mu.Lock()
	delete(g.fails, accountKey(email))
	g.mu.Unlock()
}
