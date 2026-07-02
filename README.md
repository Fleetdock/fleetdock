# db-manager

An open-source **control plane for databases** — manage a fleet of servers,
database instances, and databases from a modern dashboard instead of SSH.
Think "Vercel/Neon, but self-hosted and for any database" or "Dokploy for DB
instances". The MVP ships with **MariaDB**; the engine layer is pluggable so
postgres/mysql are cheap to add.

This is a monorepo:

```
db-manager/
  backend/           Go API + operations worker (clean architecture, Postgres metadata, JWT + RBAC)
    cmd/api          control-plane API
    cmd/agent        server agent (enrolls via install.sh, heartbeats, executes operations)
  frontend/          Next.js dashboard (React 19, TypeScript, TanStack Query)
  docker-compose.yml
```

## Quickstart (control plane)

The stack connects to a hosted **Neon** Postgres — there is no local database
container. Configuration lives in a git-ignored root `.env` file (`.env.example`
is the template); set `MDCP_DATABASE_URL` to your Neon connection string, then:

```bash
cp .env.example .env    # then edit MDCP_DATABASE_URL (skip if .env already exists)
docker compose up --build
```

- Dashboard: http://localhost:3000
- API: http://localhost:8080
- Default login: `admin@example.com` / `admin12345` (change via env before production)

The API applies its database migrations and bootstraps the admin account on first
boot. To apply migrations without starting the server: `make backend-migrate`.

## Connecting a server (one command, like Dokploy)

In the dashboard: **Servers → Connect server** generates a single-use
registration token and a ready-to-paste command:

```bash
curl -sSL https://your-control-plane/install.sh | MDCP_URL=https://your-control-plane MDCP_TOKEN=mdcpr_... sh
```

The script downloads the agent binary **from the control plane itself**
(cross-compiled into the backend image, `linux/amd64` + `linux/arm64`),
installs the MariaDB client tools, writes a systemd unit and starts the agent.
The agent enrolls, appears as a server within ~30 seconds, then heartbeats
(status, OS, memory/disk, docker, MariaDB version) every 30s and polls for
operations. Servers with no heartbeat for 2 minutes are flipped to `offline`.

Set `MDCP_PUBLIC_URL` to the URL your servers can reach the API on — it is
baked into the generated install command.

## Concepts

- **Server** — a host running the agent (connected via install.sh).
- **Instance** — a database server process. Two kinds:
  - `managed`: runs on one of your servers; operations are executed by that
    server's agent against `127.0.0.1`.
  - `external`: any reachable MariaDB (e.g. databases you already run under
    Dokploy) — the control plane connects to it directly. Add it under
    **Instances → Add instance → External**, then **Import DBs** to pull in
    the existing databases.
- **Database** — a logical database on an instance. If the instance has admin
  credentials, creating a database physically creates it (via an operation);
  otherwise it's a metadata-only registration.
- **Operation** — every async action (create/drop database, backup, restore,
  import, connection test) is a tracked job with status/progress/error,
  visible on the Operations page. Managed-instance jobs are claimed by the
  agent; external-instance jobs run on the control plane's built-in worker.
- **Backup destination** — an S3 / Cloudflare R2 / S3-compatible bucket.
  Secret keys (and instance passwords) are envelope-encrypted at rest with
  `MDCP_ENCRYPTION_KEY`.
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
- **Hardening** — login rate limiting (per client IP), security headers,
  `/healthz` + `/readyz` (DB ping) probes, and `MDCP_ENV=production` mode
  that refuses to boot with insecure default secrets.

Frontend (Next.js): login, dashboard shell, servers (connect flow with
install command), instances (external DBs, test connection, import),
databases (create, lock/unlock, delete, backup), backups (trigger, restore /
move), destinations (S3/R2 CRUD + test), operations log with live refresh,
users administration (create/edit/suspend/delete, roles, password reset),
a roles page (view every role's permissions, create/edit/delete custom roles
with a grouped permission picker), and a profile page (edit name/email,
change password) linked from the topbar.

## Local development (without Docker)

Backend (needs Go 1.22+ and a Postgres):

```bash
cd backend
export MDCP_DATABASE_URL=postgres://mdcp:mdcp@localhost:5432/mdcp?sslmode=disable
go test ./...
go run ./cmd/api
```

Agent (usually installed via install.sh; to run by hand):

```bash
go build -o db-manager-agent ./cmd/agent
MDCP_URL=http://localhost:8080 MDCP_TOKEN=<registration-token> MDCP_STATE_DIR=/tmp/dbm-agent ./db-manager-agent
```

Frontend (needs Node 22+):

```bash
cd frontend
cp .env.local.example .env.local   # NEXT_PUBLIC_API_URL=http://localhost:8080
npm install
npm run dev
```

## Configuration (backend env)

| Variable | Default | Notes |
|----------|---------|-------|
| `MDCP_DATABASE_URL` | — | required |
| `MDCP_ENV` | `development` | `production` refuses to start with default secrets |
| `MDCP_HTTP_ADDR` | `:8080` | |
| `MDCP_JWT_SECRET` | dev default | set a strong secret in production |
| `MDCP_ENCRYPTION_KEY` | dev default | encrypts credentials/S3 keys at rest; set a strong value and never rotate casually |
| `MDCP_PUBLIC_URL` | `http://localhost:8080` | URL agents/installers use to reach the API |
| `MDCP_AGENT_BIN_DIR` | `/opt/db-manager/agents` | where cross-compiled agent binaries live |
| `MDCP_WORKER_ENABLED` | `true` | in-process worker (external-instance ops, offline detection) |
| `MDCP_HEARTBEAT_TIMEOUT` | `2m` | no heartbeat for this long ⇒ server `offline` |
| `MDCP_ADMIN_EMAIL` / `MDCP_ADMIN_PASSWORD` | `admin@example.com` / `admin12345` | first-run bootstrap only |
| `MDCP_CORS_ORIGIN` | `http://localhost:3000` | frontend origin |

## Roadmap (post-MVP)

- Scheduled backups (`backup_schedules` table already exists) + retention.
- One-click "move database" composite operation (backup → restore → verify →
  cutover), building on the existing restore-to-other-instance flow.
- More engines: PostgreSQL, MySQL (implement `engine.Client`, extend the
  `instances.engine` CHECK).
- Instance provisioning (agent launches MariaDB containers via Docker).
- Audit log write path, notification channels, metrics history.
