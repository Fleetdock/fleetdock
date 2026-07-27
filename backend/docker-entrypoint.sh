#!/bin/sh
set -e

# Shared with the gateway container; HAProxy and the API must both read/write here.
mkdir -p /var/lib/fleetdock/gateway
chown -R 1000:1000 /var/lib/fleetdock/gateway
chmod 775 /var/lib/fleetdock/gateway

exec su-exec app /api "$@"
