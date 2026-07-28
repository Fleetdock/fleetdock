# Production deployment

`curl -sSL https://fleetdock.dev/install.sh | sh` covers almost every case. This
guide is for understanding what it does, and for the deployments it does not
cover.

For day-2 operations (backups, upgrades, key rotation), see [OPERATIONS.md](OPERATIONS.md).
For a security checklist before exposing the control plane, see
[SECURITY-CHECKLIST.md](SECURITY-CHECKLIST.md).

## Architecture

```text
Browser ──HTTPS──► Caddy :443 ──► fleetdock :8080
                                    ├── /v1, /agent, /healthz, /readyz,
                                    │   /docs, /openapi.yaml, /install.sh
                                    │       served in-process (Go)
                                    └── everything else
                                            proxied to the bundled dashboard
                                            (a node child on 127.0.0.1:3000)

Managed servers ──HTTPS──► the same host (install.sh + agent API)
External DB clients ──TCP──► the same host :15432-15481 (optional gateway)
```

One image, one port, one domain. The control plane stores metadata (users,
servers, credentials, jobs) in **PostgreSQL**. Managed database servers run the
**agent**, which enrolls via a one-time registration token and polls for
operations.

## Prerequisites

- A Linux host (2 vCPU, 4 GB RAM is enough for small fleets), x86_64 or arm64
- Docker Engine 24+ and **Docker Compose v2.23+**
- Ports 80 and 443 free
- A domain pointed at the host — optional; without one the installer uses an
  `<ip>.sslip.io` name that still gets a real certificate
- Outbound internet (image pulls, ACME, agent install script)

## The one-command install

```bash
curl -sSL https://fleetdock.dev/install.sh | sh -s -- --domain db.example.com
```

| Option | Effect |
| --- | --- |
| `--domain <host>` | Dashboard hostname; gets an automatic Let's Encrypt certificate |
| `--admin-email <e>` | Bootstrap admin account |
| `--dir <path>` | Install directory (default `/opt/fleetdock`) |
| `--tag <tag>` | Image tag to run (default `latest`) |
| `--with-gateway` | Also start external database access |
| `--no-tls` | Plain HTTP on the host IP; no certificate |

It installs Docker if missing, writes `/opt/fleetdock/{docker-compose.yml,.env}`,
starts the stack, and prints the dashboard URL and bootstrap password.

Re-running it upgrades in place. **It never regenerates `.env`** — rotating
`FLEETDOCK_ENCRYPTION_KEY` would make every stored credential unreadable.

## One domain, and why

Three variables name a host, and all three can be the same name:

| Variable | What reads it |
| --- | --- |
| `FLEETDOCK_PUBLIC_URL` | Agents, and the generated `install.sh` command |
| `FLEETDOCK_CORS_ORIGIN` | The API's CORS allowlist — unused on a same-origin install |
| `FLEETDOCK_GATEWAY_PUBLIC_HOST` | Baked into generated database connection URLs |

`install.sh` derives all of them from `FLEETDOCK_DOMAIN`, and `fleetdock domain`
keeps them in step afterwards.

The dashboard and the API share an origin because the Go binary serves both, so
there is no API subdomain and nothing about your hostname is compiled into the
dashboard bundle. The gateway needs a **separate** record in exactly one case:
when the main record cannot carry raw TCP — most often a Cloudflare-proxied
("orange cloud") record, which passes 80/443 only. Point a grey-clouded
`gw.example.com` at the host and set `FLEETDOCK_GATEWAY_PUBLIC_HOST` to it.

## Managing an install

```bash
fleetdock status                  # container health and readiness
fleetdock logs [service] -f
fleetdock update [--tag v0.2.0]   # pull and restart
fleetdock domain new.example.com  # move to a new hostname
fleetdock gateway enable|disable  # external database access
fleetdock doctor                  # DNS, certificates, reachability, disk
fleetdock backup-config           # archive .env — keep this off-host
fleetdock uninstall [--purge]
```

## Running it yourself

The stack is one self-contained `docker-compose.yml` with no bind mounts, so it
works from any directory next to a `.env`:

```bash
cp .env.example .env
./scripts/generate-secrets.sh >> .env
# set FLEETDOCK_DOMAIN / FLEETDOCK_SITE_ADDRESS / FLEETDOCK_PUBLIC_URL
docker compose up -d
```

To build the image from a checkout instead of pulling it, add the overlay:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d
```

Published image (multi-arch, `linux/amd64` + `linux/arm64`):

```text
ghcr.io/tajbrains/fleetdock:<tag>
```

## Installing from a checkout (forks, air-gapped, testing)

By default `install.sh` fetches `docker-compose.yml` and the `fleetdock` CLI from
the published repository and pulls the released image. Neither is available if
the repository is private, the release is not cut yet, or the host has no route
to GitHub. `--source` and `--build` cover those cases:

```bash
sudo sh install.sh --source /path/to/fleetdock --build --domain db.example.com
```

- `--source <path|url>` takes `docker-compose.yml` and `scripts/fleetdock` from a
  local checkout (or any base URL) instead of the published repo.
- `--build` builds the application image from that checkout rather than pulling
  it. It tags the image exactly as `docker-compose.yml` expects, so Compose finds
  it locally and never reaches for the registry.

### Trying it in a VM

A throwaway VM is the only honest way to test the installer, since it installs
Docker and binds 80/443. With [Multipass](https://multipass.run):

```bash
multipass launch --name fd --cpus 2 --memory 4G --disk 20G
multipass mount /path/to/fleetdock fd:/src
multipass exec fd -- sudo sh /src/scripts/install.sh --source /src --build --no-tls
```

`--no-tls` matters here: a VM on a private address cannot complete an ACME
challenge, so asking for a certificate only produces a slow failure. Reach the
dashboard at `http://<vm-ip>` — `multipass info fd` prints the address.

To test certificate issuance you need a host the internet can reach on 80/443
and a DNS record pointing at it; that is the one thing a local VM cannot cover.

## Bring your own reverse proxy

Drop the bundled `caddy` service and point your own proxy at `fleetdock:8080`.
There is no path splitting to replicate — the Go binary owns that:

```caddyfile
db.example.com {
	encode zstd gzip
	reverse_proxy fleetdock:8080
}
```

nginx equivalent:

```nginx
server {
    server_name db.example.com;
    location / {
        proxy_pass http://fleetdock:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Keep `FLEETDOCK_TRUST_PROXY_HEADERS=true` only while the API is reachable
*solely* through the proxy. If port 8080 is also exposed, a client can spoof
`X-Forwarded-For` and evade the login rate limiter.

### Split origins

Serving the dashboard and API from different hostnames is still possible but is
no longer the smooth path: `NEXT_PUBLIC_API_URL` is inlined by `next build`, so
you must set it as a build arg and build your own image, then set
`FLEETDOCK_CORS_ORIGIN` to the dashboard's origin.

## Hosted PostgreSQL

Neon, RDS, Cloud SQL, or any PostgreSQL 14+ instance works. Requirements:

- `sslmode=require` (or verify-full) for remote connections
- A dedicated database and user with DDL rights (migrations create tables)
- Regular automated backups (see [OPERATIONS.md](OPERATIONS.md))

Example connection string:

```text
postgres://fleetdock:SECRET@ep-xxx.us-east-1.aws.neon.tech/fleetdock?sslmode=require
```

## Connecting servers

In the dashboard: **Servers → Connect server**. Paste the generated command on
each database host:

```bash
curl -sSL https://dbm.example.com/install.sh | \
  sudo env FLEETDOCK_URL=https://dbm.example.com FLEETDOCK_TOKEN=fleetr_... sh
```

Put `sudo` after the pipe (before `sh`), not before `curl` — the installer must run as root.

`FLEETDOCK_PUBLIC_URL` must match the URL servers can reach (no localhost, no
internal-only DNS names unless every server is on that network). For LAN/VM dev,
use your host's LAN IP (e.g. `http://192.168.x.x:8080`).

## SMTP (optional)

Set `FLEETDOCK_SMTP_*` variables to enable email notification channels. When
`FLEETDOCK_SMTP_HOST` is empty, email delivery is disabled; Slack and webhook
channels still work.

## Resource sizing

| Fleet size    | Suggested host          | Notes                               |
| ------------- | ----------------------- | ----------------------------------- |
| 1–10 servers  | 2 vCPU, 4 GB RAM        | Default worker handles external ops |
| 10–50 servers | 4 vCPU, 8 GB RAM        | Monitor Postgres connection pool    |
| 50+ servers   | Dedicated DB + API host | Consider splitting metadata DB      |

## Troubleshooting

| Symptom                         | Likely cause                                                                                               |
| ------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `error: run as root` on install | `sudo` placed before `curl` instead of before `sh` (after the pipe)                                        |
| Install aborts on port 80/443   | nginx or apache already bound there. Stop it and re-run                                                    |
| Install aborts on Compose version | Plugin older than 2.23. Upgrade it: https://docs.docker.com/compose/install/linux/                       |
| No certificate issued           | DNS does not point at this host yet. `fleetdock doctor` compares them                                      |
| API exits on start              | `FLEETDOCK_ENV=production` with default secrets — run `generate-secrets.sh`                                |
| Agent never appears             | `FLEETDOCK_PUBLIC_URL` unreachable from the server; `localhost` used from a remote VM; firewall blocks 443 |
| Dashboard 503, API fine         | The dashboard child is restarting. `fleetdock logs fleetdock` — it recovers on its own                     |
| `/readyz` returns 503           | Metadata database down or `FLEETDOCK_DATABASE_URL` wrong                                                   |
| Database endpoints unreachable  | Gateway hostname behind a proxying CDN, or the port range not open in the host firewall                    |

Start with `fleetdock doctor`; it checks versions, container health, DNS,
reachability and disk in one pass.

Interactive API docs are available at `https://<your-host>/docs` after deploy.
