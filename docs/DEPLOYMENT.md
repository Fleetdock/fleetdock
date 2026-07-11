# Production deployment

This guide covers deploying Fleetdock on a single host with Docker Compose,
TLS termination, and the environment variables you must set for production.

For day-2 operations (backups, upgrades, key rotation), see [OPERATIONS.md](OPERATIONS.md).
For a security checklist before exposing the control plane, see
[SECURITY-CHECKLIST.md](SECURITY-CHECKLIST.md).

## Architecture

```text
Browser ──HTTPS──► reverse proxy (Caddy / nginx / Traefik)
                        ├── /        → frontend :3000
                        └── /api/*   → backend  :8080   (or same-origin /)
Managed servers ──HTTPS──► FLEETDOCK_PUBLIC_URL (install.sh + agent API)
```

The control plane stores metadata (users, servers, credentials, jobs) in
**PostgreSQL**. Managed database servers run the **agent**, which enrolls via a
one-time registration token and polls for operations.

## Prerequisites

- A Linux VM or bare-metal host (2 vCPU, 4 GB RAM is enough for small fleets)
- Docker Engine 24+ and Docker Compose v2
- A domain name with DNS pointing at the host
- TLS certificate (Let's Encrypt via Caddy is the easiest path)
- Outbound internet from the host (agent install script, optional S3 backups)

## Option A — Docker Compose (recommended)

### 1. Clone and configure

```bash
git clone https://github.com/TajBrains/fleetdock.git
cd Fleetdock
cp .env.example .env
./scripts/generate-secrets.sh >> .env
```

Edit `.env` for production:

| Variable | Example | Notes |
|----------|---------|-------|
| `FLEETDOCK_ENV` | `production` | Refuses insecure default secrets |
| `FLEETDOCK_DATABASE_URL` | `postgres://user:pass@db.example.com:5432/fleetdock?sslmode=require` | Use managed Postgres in production |
| `FLEETDOCK_JWT_SECRET` | *(from generate-secrets)* | Strong random string |
| `FLEETDOCK_ENCRYPTION_KEY` | *(from generate-secrets)* | Encrypts instance/S3 credentials at rest |
| `FLEETDOCK_ADMIN_EMAIL` | `admin@yourcompany.com` | Bootstrap admin (first boot only) |
| `FLEETDOCK_ADMIN_PASSWORD` | *(strong password)* | Change after first login |
| `FLEETDOCK_PUBLIC_URL` | `https://dbm.example.com` | URL **servers** use to reach the API |
| `FLEETDOCK_CORS_ORIGIN` | `https://dbm.example.com` | URL **browsers** load the dashboard from |
| `NEXT_PUBLIC_API_URL` | `https://dbm.example.com` | Baked into the frontend build |

**Important:** `NEXT_PUBLIC_API_URL` is embedded at **image build time**. Rebuild
the frontend image whenever this value changes.

### 2. Use the production Compose overlay

`docker-compose.prod.yml` sets `FLEETDOCK_ENV=production` and expects you to supply
secrets via `.env`. It does **not** start a local Postgres container — point
`FLEETDOCK_DATABASE_URL` at your managed database.

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml up --build -d
```

For a quick lab deploy with the bundled Postgres container, omit the prod
overlay and only set `FLEETDOCK_ENV=production` plus strong secrets in `.env`.

### 3. Verify

```bash
curl -fsS https://dbm.example.com/healthz    # {"status":"ok"}
curl -fsS https://dbm.example.com/readyz     # {"status":"ready"} (after DB is up)
```

Open the dashboard, sign in with the bootstrap admin, and change the password
on the **Profile** page.

## Option B — Pre-built images (GitHub Container Registry)

After a release is tagged (`v0.1.0`, etc.), images are published to GHCR:

```text
ghcr.io/tajbrains/fleetdock-backend:<tag>
ghcr.io/tajbrains/fleetdock-frontend:<tag>
```

Example `docker-compose.override.yml` snippet:

```yaml
services:
  backend:
    image: ghcr.io/tajbrains/fleetdock-backend:v0.1.0
    build: !reset null
  frontend:
    image: ghcr.io/tajbrains/fleetdock-frontend:v0.1.0
    build: !reset null
```

Or use the provided overlay:

```bash
export FLEETDOCK_RELEASE_TAG=v0.1.0
docker compose -f docker-compose.yml -f docker-compose.ghcr.yml up -d
```

Replace `tajbrains` with the GitHub org or user that owns the fork if you build
your own images.

## TLS and reverse proxy

### Caddy (simplest)

`deploy/Caddyfile.example`:

```caddyfile
dbm.example.com {
    encode gzip

    # API routes (adjust if you mount the API under a sub-path)
    handle /healthz* {
        reverse_proxy backend:8080
    }
    handle /readyz* {
        reverse_proxy backend:8080
    }
    handle /v1/* {
        reverse_proxy backend:8080
    }
    handle /agent/* {
        reverse_proxy backend:8080
    }
    handle /install.sh {
        reverse_proxy backend:8080
    }
    handle /openapi.yaml {
        reverse_proxy backend:8080
    }
    handle /docs* {
        reverse_proxy backend:8080
    }

    # Dashboard (everything else)
    handle {
        reverse_proxy frontend:3000
    }
}
```

Run Caddy on the host or as a fourth Compose service on ports 80/443.

### Same-origin vs split origins

| Layout | `FLEETDOCK_CORS_ORIGIN` | `NEXT_PUBLIC_API_URL` | `FLEETDOCK_PUBLIC_URL` |
|--------|--------------------|-----------------------|---------------------|
| Same host, path routing | `https://dbm.example.com` | `https://dbm.example.com` | `https://dbm.example.com` |
| API on subdomain | `https://app.example.com` | `https://api.example.com` | `https://api.example.com` |

When the API and dashboard share one origin, CORS is simpler and cookies (if
added in the future) work without cross-site configuration.

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

| Fleet size | Suggested host | Notes |
|------------|----------------|-------|
| 1–10 servers | 2 vCPU, 4 GB RAM | Default worker handles external ops |
| 10–50 servers | 4 vCPU, 8 GB RAM | Monitor Postgres connection pool |
| 50+ servers | Dedicated DB + API host | Consider splitting metadata DB |

## Troubleshooting

| Symptom | Likely cause |
|---------|----------------|
| `error: run as root` on install | `sudo` placed before `curl` instead of before `sh` (after the pipe) |
| API exits on start | `FLEETDOCK_ENV=production` with default secrets — run `generate-secrets.sh` |
| CORS errors in browser | `FLEETDOCK_CORS_ORIGIN` does not match the dashboard URL |
| Agent never appears | `FLEETDOCK_PUBLIC_URL` unreachable from the server; `localhost` used from a remote VM; firewall blocks 443 |
| Frontend calls wrong API | Rebuild frontend after changing `NEXT_PUBLIC_API_URL` |
| `/readyz` returns 503 | Metadata database down or `FLEETDOCK_DATABASE_URL` wrong |

Interactive API docs are available at `https://<your-host>/docs` after deploy.
