package dbadminapp

import (
	"testing"

	serverdom "github.com/TajBrains/fleetdock/backend/internal/domain/server"
	"github.com/TajBrains/fleetdock/backend/internal/platform/apperr"
)

func TestManagedInstanceHost_PrefersAddress(t *testing.T) {
	addr := "192.168.252.2/32"
	host, err := managedInstanceHost(&serverdom.Server{
		Hostname: "deluxe-damselfly",
		Address:  &addr,
	})
	if err != nil {
		t.Fatalf("managedInstanceHost failed: %v", err)
	}
	if host != "192.168.252.2" {
		t.Fatalf("host = %q, want %q", host, "192.168.252.2")
	}
}

func TestHostOnly(t *testing.T) {
	if got := hostOnly("192.168.252.2/32"); got != "192.168.252.2" {
		t.Fatalf("hostOnly() = %q", got)
	}
}

func TestManagedInstanceHost_MissingAddressReturnsActionableError(t *testing.T) {
	_, err := managedInstanceHost(&serverdom.Server{Hostname: "deluxe-damselfly"})
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("expected invalid, got %v", apperr.KindOf(err))
	}
}
