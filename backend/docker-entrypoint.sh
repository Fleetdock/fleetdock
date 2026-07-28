#!/bin/sh
set -e

# Shared with the gateway container; HAProxy and the API must both read/write here.
mkdir -p /var/lib/fleetdock/gateway
chown -R 1000:1000 /var/lib/fleetdock/gateway
chmod 775 /var/lib/fleetdock/gateway

# The dashboard runs as a node child of /api under the same uid. Only its
# scratch cache needs to be writable; the bundle itself stays root-owned and
# read-only.
if [ -n "${FLEETDOCK_UI_DIR:-}" ] && [ -d "${FLEETDOCK_UI_DIR}" ]; then
  mkdir -p "${FLEETDOCK_UI_DIR}/.next/cache"
  chown -R 1000:1000 "${FLEETDOCK_UI_DIR}/.next/cache" 2>/dev/null || true
fi

# exec, so /api is PID 1 and receives docker stop's SIGTERM directly; it then
# drains the HTTP server and stops the dashboard child in that order.
exec su-exec app /api "$@"
