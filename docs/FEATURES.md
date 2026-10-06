# Features and concepts

What Fleetdock does, in detail. For installing it, start with the
[README](../README.md#quick-start).

## Concepts

The dashboard uses plain words for these; [glossary.md](glossary.md)
maps them to the names in the code and API.

- **Server** — a host running the agent (connected via install.sh).
- **Instance** (_database server_ in the dashboard) — a database server
  process. Two kinds:
  - `managed`: runs on one of your servers; operations are executed by that
    server's agent against `127.0.0.1`. A managed instance can either be
    **provisioned** by Fleetdock (the agent launches a MariaDB **Docker
    container** with a generated root password, a named data volume and a
    published port — **Databases → Connect database server → Create a new one**) or
    **registered** (point at a MariaDB already running on the server).
  - `external`: any reachable database you already host elsewhere — the control
    plane connects to it directly. Add it under
    **Databases → Connect database server → Connect to one anywhere**.
    Provisioned instances can be **started / stopped / restarted** from their
    detail page; deleting one removes the container (and, if you confirm, its
    data volume). Provisioning needs Docker on the server — `install.sh`
    installs it automatically.
- **Database** — a logical database on an instance. If the instance has admin
  credentials, creating a database physically creates it (via an operation);
  otherwise it's a metadata-only registration.
- **Discovery** — every minute Fleetdock checks each instance it has
  credentials for and syncs its database list: new databases appear on their
  own, ones dropped outside Fleetdock are marked _not found on server_ after
  about three minutes and return when they reappear. Nothing is deleted.
  **Check now** on an instance runs the check immediately.
- **Operation** — every async action (create/drop database, backup, restore,
  import, connection test) is a tracked job with status/progress/error,
  visible on the **Activity** page. Managed-instance jobs are claimed by the
  agent; external-instance jobs run on the control plane's built-in worker.
- **Backup destination** (_backup storage_) — an S3 / Cloudflare R2 /
  S3-compatible bucket.
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
- **Live DB administration** — for **MariaDB, MySQL, and PostgreSQL**:
  - accounts and grants: list/create/drop users (MariaDB `user@host`,
    PostgreSQL roles), set passwords, view grants, grant/revoke schema
    privileges (allowlisted catalog at `GET /v1/db-privileges`);
  - data: filtered, sorted, searchable browser; insert/edit/delete rows by
    primary key (exactly one row or nothing); CSV import and export; SQL file
    import;
  - structure without SQL: create/alter/rename/truncate/drop tables, columns,
    indexes and foreign keys; views, routines, triggers, sequences and events;
  - SQL console: multi-statement scripts on one session, confirmation before
    writes, cancel, per-user history and saved queries;
  - monitoring: live sessions (cancel/terminate), status counters,
    configuration; a per-minute health probe fills instance health and
    database sizes and connection counts.

  Everything that touches one database's data runs as a **Fleetdock-managed
  role confined to that database** (`fleetdock_ro_*` / `fleetdock_rw_*`), never
  as the instance admin — see [Security](#security). Executed synchronously by
  the control plane: external instances are reached at their host, managed instances at their server's
  address (reported automatically by the agent on enroll/heartbeat). The
  instance DB port must be reachable from the control plane — for LAN/VM dev,
  ensure published ports on the server are open to the host running Fleetdock.
- **Backups** — also downloadable (presigned link), deletable, and verifiable by
  a test restore into a scratch database.
- **Hardening** — login rate limiting (per client IP), security headers and CSP,
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
databases (info, tables with a data browser and row editor, structure editor,
views/routines, SQL console, per-database users & grants with grant/revoke).

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

## Security

- **Production mode:** set `FLEETDOCK_ENV=production` and provide strong values for
  `FLEETDOCK_JWT_SECRET`, `FLEETDOCK_ENCRYPTION_KEY`, and `FLEETDOCK_ADMIN_PASSWORD`. The API
  refuses to start if defaults are still in use.
- **Least privilege in the database:** the console, browser, exports, data and
  structure editing and SQL imports connect as per-database roles created by
  Fleetdock. The engine itself confines them: other databases, account tables,
  server files and global settings are unreachable whatever SQL is typed.
  The engine-owned `mysql`/`sys`/`postgres` databases can only be opened by
  users with `instance:write`. Root credentials are used only for
  instance-level administration (accounts, grants, create/drop database,
  monitoring) — keep `instance:write` for administrators.
- **Network:** instance hosts are checked when saved and when dialled —
  loopback (in production), link-local, cloud-metadata and the metadata
  database itself are refused. Choose `tls_mode` per instance; use
  `verify-full` across untrusted networks.
- **JWT storage:** the dashboard stores the session JWT in `localStorage`. A
  Content-Security-Policy confines the dashboard to its own origin, so injected
  script cannot send the token elsewhere, and signing out revokes all of the
  user's sessions server-side. Deploy on trusted networks and keep the frontend
  dependency supply chain clean.
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

- **Vulnerability reports:** see [SECURITY.md](../SECURITY.md).

## API documentation

The HTTP API is documented by a hand-authored OpenAPI 3 spec at
[`backend/internal/openapi/openapi.yaml`](../backend/internal/openapi/openapi.yaml).
It is embedded in the binary and served at **`GET /openapi.yaml`**, with an
interactive Redoc page at **`GET /docs`** (both public). A unit test keeps the
spec in sync with the routes defined in `router.go`.
