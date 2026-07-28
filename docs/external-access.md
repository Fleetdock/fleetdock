# External database access

Fleetdock can expose managed databases to applications outside the private network through a **Fleetdock Gateway** (HAProxy). The control plane generates configuration from persisted endpoint state; HAProxy proxies TCP traffic, one public port per database.

## Architecture

```
External app → gateway.example.com:15432 (TCP) → HAProxy → server.address:instance.port → Database
```

The API never proxies database connections. Desired state lives in `database_endpoints`; the HAProxy config is derived from it and reconciled by the control-plane worker.

The gateway container must be able to reach `server.address:port`. This is the same requirement as live database administration and backups of external instances.

## Enable the gateway

External access is **off by default** — enabling it publishes a 50-port range on
the host, which most installs never need.

```bash
fleetdock gateway enable
```

That sets `FLEETDOCK_GATEWAY_ENABLED=true`, activates the `gateway` compose
profile (HAProxy plus its one-shot init, sharing `/var/lib/fleetdock/gateway`
with the control plane), and prints the port range to open in your firewall.
Then, on the database page → **Connectivity** → **Enable public access**, with
the networks allowed to connect.

By hand, the equivalent is `FLEETDOCK_GATEWAY_ENABLED=true` in `.env` plus
`docker compose --profile gateway up -d`.

### Hostname

`FLEETDOCK_GATEWAY_PUBLIC_HOST` defaults to your main `FLEETDOCK_DOMAIN`. The
gateway speaks raw TCP on its own ports — no vhost, no SNI, no TLS termination —
so it needs no DNS record of its own.

Set it separately in one case: when the main record cannot carry raw TCP. The
usual culprit is a **Cloudflare-proxied ("orange cloud") record**, which forwards
80/443 only, so every database endpoint on that name is unreachable. Point a
grey-clouded `gw.example.com` at the host and set the variable to it.

The hostname is copied into each endpoint when public access is enabled, so
changing it later does not rewrite connection URLs already handed out — see
`fleetdock domain`.

The published port range and the allocator range come from the same variables. If you widen one, widen the other — otherwise endpoints get ports that nothing forwards, and only external clients notice.

## Source IP fidelity — read this before setting an allowlist

Allowlists match **the source address HAProxy observes**, which is not always your client's address.

| Deployment | Address HAProxy sees | Allowlists work? |
|---|---|---|
| Linux host, published ports (iptables DNAT) | the real client address | Yes |
| Docker Desktop (macOS/Windows) | the Docker VM's NAT address (e.g. `192.168.65.1`) | **No** — every real client is rejected |
| Behind an L4 load balancer | the balancer's address | Only with `FLEETDOCK_GATEWAY_SOURCE_IP_MODE=proxy-protocol` and PROXY protocol enabled on the balancer |
| `network_mode: host` on Linux | the real client address | Yes, and avoids publishing a large port range |

**Find out what your gateway actually sees.** The gateway serves a diagnostic on `FLEETDOCK_GATEWAY_DIAG_PORT` (default `15431`). Run this **from the machine that will connect**:

```
curl http://gateway.example.com:15431
```

It prints the address to allowlist. Set the diag port to `0` to disable it.

A bare address is accepted and treated as a single host (`/32` or `/128`). A CIDR with host bits set — `10.1.2.3/24` — is rejected rather than silently widened to a whole network you did not ask for.

## What Fleetdock can and cannot verify

| Fact | How it is determined |
|---|---|
| The route is programmed and the gateway can reach the database | HAProxy's own health check, read over the stats socket |
| The database accepts TLS | The control plane dials the database **directly** and performs a real protocol probe (Postgres `SSLRequest`, MySQL/MariaDB `CLIENT_SSL` capability) |
| The published port is reachable from the internet | **Not determinable.** Firewalls, NAT, and DNS sit outside the control plane. |

Endpoint status reflects those facts:

- `pending` — not yet present in the applied gateway config, or the last apply failed (see `last_error`)
- `active` — in the applied config, and HAProxy reports the database reachable
- `error` — in the applied config, but HAProxy reports the database down. The listener stays up so a database restart does not reallocate the port.
- `disabling` / `disabled` — being removed / removed

`tls_status` is a property of the **database**, not the gateway. When it reports `unsupported` while `tls_mode` is `required`, issued connection URLs carry `sslmode=require` and will fail until TLS is enabled on the database. Fleetdock does not quietly downgrade the requirement.

## Changing the allowlist

Edit it in place from the connectivity panel. **The port is preserved**, so existing clients keep working. Disabling and re-enabling public access allocates a different port and breaks every connection URL already handed out.

If the panel shows rejected connections with no successful sessions, the allowlist does not match where clients are arriving from — check the diagnostic port above.

## Application credentials

Create scoped credentials under **Application credentials**. Passwords are encrypted at rest; the connection URL, CLI command, and password are shown once on create and rotate.

Permission profiles: `readonly`, `readwrite`, `admin` (database-scoped). Leave the username blank to have one generated from the credential name.

Revoking a credential disables the database account and closes its open sessions. Where the account still owns objects, the account is left in a no-login state rather than dropped — access ends either way.

## TLS

HAProxy uses TCP passthrough; it does not terminate TLS. Clients negotiate TLS with the database directly, so the database must support it for `sslmode=require` to work.

## Security

- Public access is **disabled by default**
- CIDR allowlists are enforced in HAProxy (`tcp-request connection reject unless …`), subject to the source-IP caveats above
- An endpoint with an empty allowlist rejects everything — the configuration fails closed
- `0.0.0.0/0` must be entered explicitly and is confirmed twice in the UI
- Instance root credentials are never exposed for public use

## Troubleshooting

| Symptom | Check |
|---|---|
| Endpoint stuck `pending` | `last_error` on the endpoint; the `reconcile_gateway` operation; whether the gateway container is healthy |
| Endpoint shows `error` | `last_error` names the unreachable `host:port`. Can the **gateway container** reach it? |
| Connections are refused immediately | The allowlist. Run `curl http://<public-host>:15431` from the connecting machine and compare. The panel also reports rejected-connection counts. |
| `server does not support SSL, but SSL was required` | The database does not accept TLS. Enable it, or recreate the endpoint with `tls_mode=preferred`. |
| Cannot connect at all | DNS for `FLEETDOCK_GATEWAY_PUBLIC_HOST`, host firewall on the port range, and that the range is actually published |

## Limitations

- No TLS termination at the gateway (passthrough only)
- One public port per database; no hostname/SNI routing
- The gateway must have a network route to `server.address:port`
- Integration tests that need a live HAProxy are skipped without Docker

## Production

Open the gateway port range in the host firewall — the control plane cannot verify reachability from outside, so this is the step that most often gets missed. On Linux, `network_mode: host` for the gateway service both avoids publishing a large port range and preserves real client addresses.
