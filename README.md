# Fleetdock

[![CI](https://github.com/Fleetdock/fleetdock/actions/workflows/ci.yml/badge.svg)](https://github.com/Fleetdock/fleetdock/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](backend/go.mod)
[![Website](https://img.shields.io/badge/website-fleetdock.dev-0ea5e9)](https://fleetdock.dev)

An open-source **control plane for your database fleet** — manage servers,
database instances, and databases from a modern dashboard instead of SSH.
Think self-hosted database operations with a modern dashboard — like Vercel/Neon,
but for any database engine you run. It supports **MariaDB, MySQL and PostgreSQL** behind a pluggable
engine layer, and can **provision** new database containers on your servers via
plain Docker (no Swarm/Traefik).

Licensed under [Apache-2.0](LICENSE).

**Who is this for?** Teams and individuals who run MariaDB, MySQL, or PostgreSQL
on their own servers and want a single dashboard for fleet health, backups,
provisioning, and day-to-day admin — without handing database credentials to a
SaaS vendor.

> Screenshots: see [docs/screenshots/](docs/screenshots/). Replace placeholders with real captures from a running stack before major launch posts.

| In 60 seconds        |                                                                                                                                                                              |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Install**          | `curl -sSL https://fleetdock.dev/install.sh \| sh` — one command, one domain                                                                                                 |
| **Connect a server** | One `curl … install.sh` command from the dashboard                                                                                                                           |
| **Manage**           | Instances, databases, backups, users/grants, operations log                                                                                                                  |
| **Docs**             | [Deploy to production](docs/DEPLOYMENT.md) · [Operations](docs/OPERATIONS.md) · [Security checklist](docs/SECURITY-CHECKLIST.md) · [API `/docs`](http://localhost:8080/docs) |

This is a monorepo:

```
fleetdock/
  backend/           Go API + operations worker (clean architecture, Postgres metadata, JWT + RBAC)
    cmd/api          control-plane API; also fronts and supervises the dashboard
    cmd/agent        server agent (enrolls via install.sh, heartbeats, executes operations)
  frontend/          Next.js dashboard (React 19, TypeScript, TanStack Query)
  scripts/           install.sh (control plane installer) and the fleetdock CLI
  docs/              Deployment, operations, security guides
  Dockerfile         One image: control plane + dashboard
  docker-compose.yml Caddy + Fleetdock + Postgres (+ optional gateway)
```

## Install

On any Linux host with a domain pointed at it:

```bash
curl -sSL https://fleetdock.dev/install.sh | sh -s -- --domain db.example.com
```

Without `--domain`, the installer uses an `<ip>.sslip.io` name so you still get a
real TLS certificate with no DNS setup at all:

```bash
curl -sSL https://fleetdock.dev/install.sh | sh
```

It installs Docker if needed, generates secrets, starts the stack behind Caddy
with automatic HTTPS, and prints the dashboard URL and bootstrap password. Only
ports **80** and **443** are published.

Day-2 operations use the `fleetdock` command it installs:

```bash
fleetdock status          # container health and readiness
fleetdock logs            # tail everything
fleetdock update          # pull a new release and restart
fleetdock doctor          # diagnose DNS, certificates, reachability
fleetdock domain new.example.com
fleetdock gateway enable  # external database access (opt-in)
```

**One domain is all you need.** The Go binary serves the API and the dashboard
on the same origin and the same port, so there is no separate API hostname and
nothing about your domain is baked into any image. The external-access gateway
speaks raw TCP on its own ports and reuses the same hostname.

**Hosted Postgres:** point `FLEETDOCK_DATABASE_URL` at any PostgreSQL 14+ with
`sslmode=require` and scale the bundled one away:
`docker compose up -d --scale postgres=0`.

## Run from source

```bash
cp .env.example .env
./scripts/generate-secrets.sh >> .env
docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d
```

Or `make up`. The dashboard is at the address in `FLEETDOCK_SITE_ADDRESS`
(`http://localhost` by default). The API applies its migrations and bootstraps
the admin account on first boot; `make backend-migrate` applies them without
starting the server.

## Connecting a server (one command)

In the dashboard: **Servers → Connect server** generates a single-use
registration token and a ready-to-paste command:

```bash
curl -sSL https://your-control-plane/install.sh | \
  sudo env FLEETDOCK_URL=https://your-control-plane FLEETDOCK_TOKEN=fleetr_... sh
```

Put `sudo` after the pipe (before `sh`), not before `curl` — the installer must run as root to write systemd units and `/usr/local/bin`.

The script downloads the agent binary **from the control plane itself**
(cross-compiled into the backend image, `linux/amd64` + `linux/arm64`),
installs the MariaDB client tools, writes a systemd unit and starts the agent.
The agent enrolls, appears as a server within ~30 seconds, then heartbeats
(status, OS, memory/disk, docker, MariaDB version) every 30s and polls for
operations. Servers with no heartbeat for 2 minutes are flipped to `offline`.

Set `FLEETDOCK_PUBLIC_URL` to the URL your servers can reach the API on — it is
baked into the generated install command. For LAN/VM development (e.g. control
plane on your Mac, agent on a Ubuntu VM), use your host's LAN IP instead of
`localhost`:

```bash
FLEETDOCK_PUBLIC_URL=http://192.168.x.x:8080
```

## Concepts

- **Server** — a host running the agent (connected via install.sh).
- **Instance** — a database server process. Two kinds:
  - `managed`: runs on one of your servers; operations are executed by that
    server's agent against `127.0.0.1`. A managed instance can either be
    **provisioned** by Fleetdock (the agent launches a MariaDB **Docker
    container** with a generated root password, a named data volume and a
    published port — Servers → _server_ → **Add instance → Provision new**) or
    **registered** (point at a MariaDB already running on the server).
  - `external`: any reachable database you already host elsewhere — the control
    plane connects to it directly. Add it under
    **Instances → Add instance → External**, then **Import DBs** to pull in
    the existing databases.
    Provisioned instances can be **started / stopped / restarted** from their
    detail page; deleting one removes the container (and, if you confirm, its
    data volume). Provisioning needs Docker on the server — `install.sh`
    installs it automatically.
- **Database** — a logical database on an instance. If the instance has admin
  credentials, creating a database physically creates it (via an operation);
  otherwise it's a metadata-only registration.
- **Operation** — every async action (create/drop database, backup, restore,
  import, connection test) is a tracked job with status/progress/error,
  visible on the Operations page. Managed-instance jobs are claimed by the
  agent; external-instance jobs run on the control plane's built-in worker.
- **Backup destination** — an S3 / Cloudflare R2 / S3-compatible bucket.
  Secret keys (and instance passwords) are envelope-encrypted at rest with
  `FLEETDOCK_ENCRYPTION_KEY`.
- **Backup / Restore** — `mariadb-dump | gzip`, streamed to the bucket via
  presigned URLs (agents never see storage credentials). Restoring into a
  different instance and/or database name is how you **move a database to
  another server**.

## What's implemented

Backend (Go):

- **Auth** — email/password login issuing JWTs, `/auth/me`, bcrypt hashing.
- **RBAC** — permission-based middleware; roles (`owner`, `admin`, `operator`,
  `viewer`) seeded with a permission catalog (incl. `operation:*`, `backup:*`,
  `destination:*`).
- **API tokens** — create / list / revoke; scoped like sessions.
- **Servers & agents** — one-command install, single-use registration tokens,
  agent enrollment + bearer-token auth, heartbeats + health snapshots,
  offline detection.
- **Instances** — managed & external, engine field (mariadb now, pluggable
  registry for more), encrypted admin credentials, connection test, import
  of existing databases.
- **Databases** — create (physical when credentials exist) / list / lock /
  unlock / soft-delete (7-day recovery window).
- **Operations engine** — jobs table with `FOR UPDATE SKIP LOCKED` claiming,
  payload enrichment (credentials + presigned URLs) at claim time, completion
  side effects, control-plane worker for external instances.
- **Backups** — destinations (S3/R2/S3-compatible) with encrypted secrets and
  a "test bucket" action; manual backups (`mariadb-dump`, gzip, sha256,
  size); restore to any instance/name (= move).

- **Users & roles** — full account administration API: list/create/edit/
  suspend/delete users, assign global roles, admin password reset. Custom
  roles: create/edit/delete roles with any subset of the permission catalog
  (`GET /v1/permissions`); system roles (`owner`/`admin`/`operator`/`viewer`)
  are immutable, and roles still assigned to users cannot be deleted. Guards
  prevent lockout: the last active owner cannot be demoted, suspended or
  deleted, and you cannot suspend or delete yourself.
- **Profile** — self-service name/email update and password change (requires
  the current password). Suspended accounts cannot log in and existing
  sessions/tokens stop working immediately.
- **Live DB administration** — database accounts and grants managed straight
  from the dashboard for **MariaDB, MySQL, and PostgreSQL**: list/create/drop
  users (MariaDB `user@host`, PostgreSQL roles), view grants, grant/revoke
  schema privileges (allowlisted catalog at `GET /v1/db-privileges`); plus
  table listing, paginated data browser, SQL console, and CSV export per
  database. Executed synchronously by the control plane: external instances are reached at their host, managed instances at their server's
  address (reported automatically by the agent on enroll/heartbeat). The
  instance DB port must be reachable from the control plane — for LAN/VM dev,
  ensure published ports on the server are open to the host running Fleetdock.
- **Hardening** — login rate limiting (per client IP), security headers,
  `/healthz` (liveness) and `/readyz` (metadata DB ping) probes, and
  `FLEETDOCK_ENV=production` mode that refuses to boot with insecure default secrets.

Frontend (Next.js): login, dashboard shell, servers (connect flow with
install command), instances (external DBs, test connection, import),
databases (create, lock/unlock, delete, backup), backups (trigger, restore /
move), destinations (S3/R2 CRUD + test), operations log with live refresh,
users administration (create/edit/suspend/delete, roles, password reset),
a roles page (view every role's permissions, create/edit/delete custom roles
with a grouped permission picker), and a profile page (edit name/email,
change password) linked from the topbar. Detail pages: instances (info,
databases, live DB users with expandable grants, create/drop/grant) and
databases (info, tables with a paginated row browser, per-database users &
grants with grant/revoke).

## Local development (without Docker)

Backend (needs Go 1.25+ and a Postgres):

```bash
cd backend
export FLEETDOCK_DATABASE_URL=postgres://fleetdock:fleetdock@localhost:5432/fleetdock?sslmode=disable
go test ./...
go run ./cmd/api
```

Or use `make dev` from the repo root (loads `.env` if present).

Agent (usually installed via install.sh; to run by hand):

```bash
go build -o fleetdock-agent ./cmd/agent
FLEETDOCK_URL=http://localhost:8080 FLEETDOCK_TOKEN=<registration-token> FLEETDOCK_STATE_DIR=/tmp/fleetdock-agent ./fleetdock-agent
```

Frontend (needs Node 22+):

```bash
cd frontend
npm install
npm run dev
```

Open http://localhost:3000. The dev server proxies `/v1`, `/agent` and the other
API paths to the Go API on `:8080`, so the dashboard is same-origin in
development exactly as it is in production. Point it elsewhere with
`FLEETDOCK_DEV_API_URL`.

## Configuration (backend env)

Environment variables use the `FLEETDOCK_*` prefix.

| Variable                                              | Default                 | Notes                                                                                                             |
| ----------------------------------------------------- | ----------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `FLEETDOCK_DATABASE_URL`                              | —                       | required                                                                                                          |
| `FLEETDOCK_ENV`                                       | `development`           | `production` refuses to start with default secrets                                                                |
| `FLEETDOCK_HTTP_ADDR`                                 | `:8080`                 |                                                                                                                   |
| `FLEETDOCK_JWT_SECRET`                                | dev default             | set a strong secret in production                                                                                 |
| `FLEETDOCK_ENCRYPTION_KEY`                            | dev default             | primary key that encrypts credentials/S3 keys at rest; rotate via `make rotate-keys` (see Security)               |
| `FLEETDOCK_ENCRYPTION_KEY_ID`                         | `master-1`              | id stamped on secrets wrapped by the primary key; use a new id when rotating                                      |
| `FLEETDOCK_ENCRYPTION_KEYS_OLD`                       | —                       | retired keys still needed to decrypt during rotation, as `id=secret,id2=secret2`                                  |
| `FLEETDOCK_PUBLIC_URL`                                | `http://localhost:8080` | URL agents/installers use to reach the API                                                                        |
| `FLEETDOCK_AGENT_BIN_DIR`                             | `/opt/fleetdock/agents` | where cross-compiled agent binaries live                                                                          |
| `FLEETDOCK_WORKER_ENABLED`                            | `true`                  | in-process worker (external-instance ops, offline detection, scheduled backups, retention, alerts, notifications) |
| `FLEETDOCK_HEARTBEAT_TIMEOUT`                         | `2m`                    | no heartbeat for this long ⇒ server `offline`                                                                     |
| `FLEETDOCK_METRICS_RETENTION`                         | `168h`                  | how long per-heartbeat server metrics history is kept                                                             |
| `FLEETDOCK_SMTP_HOST`                                 | —                       | SMTP host for email notification channels (empty ⇒ email delivery disabled)                                       |
| `FLEETDOCK_SMTP_PORT`                                 | `587`                   | SMTP port                                                                                                         |
| `FLEETDOCK_SMTP_USERNAME` / `FLEETDOCK_SMTP_PASSWORD` | —                       | SMTP auth (optional)                                                                                              |
| `FLEETDOCK_SMTP_FROM`                                 | `fleetdock@localhost`   | envelope/from address for emails                                                                                  |
| `FLEETDOCK_ADMIN_EMAIL` / `FLEETDOCK_ADMIN_PASSWORD`  | —                       | first-run bootstrap only; generate with `./scripts/generate-secrets.sh`                                           |
| `FLEETDOCK_CORS_ORIGIN`                               | `http://localhost:3000` | only used for split-origin deployments; same-origin installs never hit CORS                                       |
| `FLEETDOCK_UI_DIR`                                    | —                       | the bundled dashboard. Set by the image; empty means API-only (bare binary, dev)                                  |
| `FLEETDOCK_UI_PORT`                                   | `3000`                  | loopback port the dashboard binds; never published                                                                |

## API documentation

The HTTP API is documented by a hand-authored OpenAPI 3 spec at
[`backend/internal/openapi/openapi.yaml`](backend/internal/openapi/openapi.yaml).
It is embedded in the binary and served at **`GET /openapi.yaml`**, with an
interactive Redoc page at **`GET /docs`** (both public). A unit test keeps the
spec in sync with the routes defined in `router.go`.

## Security

- **Production mode:** set `FLEETDOCK_ENV=production` and provide strong values for
  `FLEETDOCK_JWT_SECRET`, `FLEETDOCK_ENCRYPTION_KEY`, and `FLEETDOCK_ADMIN_PASSWORD`. The API
  refuses to start if defaults are still in use.
- **JWT storage:** the dashboard stores the session JWT in `localStorage`. This
  is convenient but exposes the token to XSS — deploy only on trusted networks
  and keep the frontend dependency supply chain clean.
- **Encryption key rotation:** secrets are envelope-encrypted, so rotating the
  master key only re-wraps each secret's data key — the payload ciphertext is
  never touched. To rotate, keep the old key readable and promote a new key with
  a **new id**, then run the offline re-wrap:

  ```sh
  export FLEETDOCK_ENCRYPTION_KEYS_OLD="master-1=<old-secret>"   # keep old key readable
  export FLEETDOCK_ENCRYPTION_KEY="<new-secret>"                 # new primary secret
  export FLEETDOCK_ENCRYPTION_KEY_ID="master-2"                  # new primary id
  make rotate-keys                                          # re-wrap every secret
  # once it reports 0 remaining, drop FLEETDOCK_ENCRYPTION_KEYS_OLD and keep the new key
  ```

  Rotation requires a new key id — reusing the same id with a different secret
  would leave existing secrets unreadable.

- **Vulnerability reports:** see [SECURITY.md](SECURITY.md).

## Automation & observability

- **Overview dashboard** — fleet health, instance/database counts, backup and
  operation status, and automation summary at a glance (`GET /v1/overview`).
- **Scheduled backups** — cron-scheduled recurring backups per database with a
  retention window; the worker enqueues them and prunes expired backups (object
  - metadata). Managed from the **Schedules** page.
- **Notifications & alerts** — email / Slack / webhook channels and alert rules
  on server metrics (CPU, memory %, disk %, connections). Backup failures and
  offline servers notify automatically; the worker evaluates rules and delivers
  events via a transactional outbox. Managed on the **Notifications** page.
- **Metrics history** — every heartbeat is stored as a time-series sample and
  charted (CPU/memory/disk/connections) on each server's detail page
  (`GET /v1/servers/{id}/metrics`).

## Move & verify

- **Move database** — a background saga (`backup → restore → verify → optional
drop of source`) copies or relocates a database to another instance, across
  servers. Start it from a database's detail page ("Move"); watch it on the
  **Moves** page. Ticking "drop source" makes it a true move (cutover); leaving
  it unticked makes it a copy.
- **Restore verification** — every restore verifies the backup artifact's
  sha256 checksum _before_ touching the target, then counts the restored tables
  and reports the count.

## Production deployment

See **[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)** for TLS, reverse proxy, hosted
Postgres, and environment variables. Day-2 operations (backups, upgrades, key
rotation) are in **[docs/OPERATIONS.md](docs/OPERATIONS.md)**. Run through
**[docs/SECURITY-CHECKLIST.md](docs/SECURITY-CHECKLIST.md)** before exposing the
control plane to the internet.

A single multi-arch image is published to GitHub Container Registry on each
version tag, containing both the control plane and the dashboard:

```text
ghcr.io/fleetdock/fleetdock:<tag>     # linux/amd64, linux/arm64
```

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for development
setup, code style, and pull request guidelines.

- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Security policy](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [Roadmap](ROADMAP.md)
- [Releasing](RELEASING.md)

## Roadmap

See **[ROADMAP.md](ROADMAP.md)** for the full phased plan. Highlights still on
the horizon:

- SSO/OIDC login and optional 2FA/MFA
- Secure external DB access without opening ports (Cloudflare Tunnel / TCP proxy)
- Composite/HA topologies (replicas)
- Expanded integration test coverage
