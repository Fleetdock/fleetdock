package engine

import (
	"testing"
	"time"
)

// A long statement must not be cut off by the driver's per-read deadline.
func TestMySQLReadTimeoutOutlastsStatement(t *testing.T) {
	m := &MariaDB{}
	if got := m.config(ConnParams{}).ReadTimeout; got != 30*time.Second {
		t.Fatalf("default read timeout = %s", got)
	}
	if got := m.config(ConnParams{StatementTimeout: 10 * time.Minute}).ReadTimeout; got <= 10*time.Minute {
		t.Fatalf("read timeout %s does not outlast a 10 minute statement", got)
	}
}
