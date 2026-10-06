// Package dbtarget resolves how the control plane reaches a database instance.
//
// Host resolution was previously reimplemented in the endpoint, credential, and
// dbadmin services with subtly different rules — two of them silently accepted a
// server with no address, producing an empty host that surfaced much later as a
// confusing connection error. This is the single implementation.
package dbtarget

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	instancedom "github.com/Fleetdock/fleetdock/backend/internal/domain/instance"
	serverdom "github.com/Fleetdock/fleetdock/backend/internal/domain/server"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/netsafe"
)

// Servers is the subset of the server repository host resolution needs.
type Servers interface {
	GetByID(ctx context.Context, id uuid.UUID) (*serverdom.Server, error)
}

// Host returns the address the control plane should dial for an instance.
//
// field names the request field blamed in validation errors, so each caller
// keeps reporting the field its own API exposes.
//
// The result is checked against the netsafe database policy: instance hosts
// (and the addresses agents report for their servers) are untrusted input.
func Host(ctx context.Context, servers Servers, inst *instancedom.Instance, field string) (string, error) {
	host, err := resolveHost(ctx, servers, inst, field)
	if err != nil {
		return "", err
	}
	if err := CheckHost(ctx, host, inst.Port, field); err != nil {
		return "", err
	}
	return host, nil
}

// CheckHost validates a user- or agent-supplied database address against the
// netsafe policy, reporting a violation as a validation error on field.
func CheckHost(ctx context.Context, host string, port int, field string) error {
	if err := netsafe.CheckDBHost(ctx, hostOnly(host), port); err != nil {
		if errors.Is(err, netsafe.ErrDisallowedDBHost) {
			return apperr.Invalid(field,
				"this host is not allowed: loopback, link-local, cloud-metadata and control-plane addresses are blocked")
		}
		return apperr.Internal(err)
	}
	return nil
}

func resolveHost(ctx context.Context, servers Servers, inst *instancedom.Instance, field string) (string, error) {
	switch {
	case inst.Kind == instancedom.KindExternal && inst.Host != nil && *inst.Host != "":
		return hostOnly(*inst.Host), nil

	case inst.ServerID != nil:
		srv, err := servers.GetByID(ctx, *inst.ServerID)
		if err != nil {
			return "", err
		}
		return managedHost(srv, field)

	default:
		return "", apperr.Invalid(field, "cannot determine a reachable host for this instance")
	}
}

func managedHost(srv *serverdom.Server, field string) (string, error) {
	if srv.Address != nil && *srv.Address != "" {
		return hostOnly(*srv.Address), nil
	}
	if srv.Hostname != "" {
		return "", apperr.Invalid(field,
			"server has no reachable address; ensure the agent reports its IP (upgrade agent) or set the server address manually")
	}
	return "", apperr.Invalid(field, "cannot determine a reachable host for this instance")
}

// hostOnly strips a CIDR suffix from an address the agent may report as "10.0.0.5/24".
func hostOnly(addr string) string {
	if i := strings.Index(addr, "/"); i >= 0 {
		return addr[:i]
	}
	return addr
}
