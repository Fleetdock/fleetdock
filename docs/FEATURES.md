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
  - undo: every insert, edit and delete made in the data browser is kept in a
    change journal with the row before and after (for
    `FLEETDOCK_ROW_CHANGE_RETENTION`, a week by default). **Undo** right after
    saving, or later from a table's **Change history**, per row or per save;
    if the row changed since, Fleetdock asks before overwriting that change.
    Undo does not revert what triggers or cascading deletes did, and changes
    made in the SQL console or by CSV import are not journaled;
  - structure without SQL: create/alter/rename/truncate/drop tables, columns,
    indexes and foreign keys; views, routines, triggers, sequences and events;
  - SQL console: multi-statement scripts on one session, confirmation before
    writes, cancel, per-user history and saved queries;
  - monitoring: live sessions (cancel/terminate), the most expensive
    statements (`pg_stat_statements` / `performance_schema`), status
    counters, configuration; a per-minute health probe fills instance health
    and database sizes and connection counts;
  - performance: the console's **Explain** draws a statement's plan as a
    tree (nothing runs) and **Analyze** runs a read in a rolled-back,
    read-only transaction to show where the time went, flagging full scans,
    filesorts and stale statistics; **Index health** lists unused indexes
    and indexes made redundant by another one, with the `DROP INDEX` to
    review — Fleetdock never drops them itself;
  - navigation: the SQL editor autocompletes the database's real tables and
    columns; foreign keys (composite ones too) link to the referenced row,
    and a row's menu opens the rows of other tables that reference it;
    Ctrl/Cmd+K finds a table or column by name across every database of
    the fleet (only databases you may open are shown).

  The login that the data browser, SQL console, exports and SQL imports use
  is chosen per instance (**Data browsing connects as**, in the instance's
  settings):
  - **Admin login** (default) — the instance's admin login, like pgAdmin or
    DBeaver. It reaches every database, so only instance administrators
    (`instance:write`) may use these features on that instance;
  - **Dedicated login** — a login you create on the server (e.g. read-only);
    the server's grants decide what it can do, and anyone with access to a
    database may browse it;
  - **Fleetdock-managed roles** — Fleetdock creates a role confined to each
    database (`fleetdock_ro_*` / `fleetdock_rw_*`) on first use and repairs it
    if it is dropped or its password changed outside Fleetdock.

  System databases (`mysql`, `postgres`, …) always require instance
  administrator rights — see [Security](#security). Executed synchronously by
  the control plane: external instances are reached at their host, managed instances at their server's
  address (reported automatically by the agent on enroll/heartbeat). The
  instance DB port must be reachable from the control plane — for LAN/VM dev,
  ensure published ports on the server are open to the host running Fleetdock.
- **Backups** — also downloadable (presigned link), deletable, and verifiable by
  a test restore into a scratch database.
- **One-off backup download** — a database can be dumped straight to the
  browser (`.sql.gz`) without any backup storage. The control plane runs the
  dump (it must reach the database port, as for the data browser) and keeps
  nothing; the file can be restored with Import SQL.
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
- **Who may browse data:** the console, browser, exports, data and structure
  editing and SQL imports connect with the login the instance is set to use.
  With its **admin login** (the default) they reach every database on the
  instance, so only users with `instance:write` on it may use them. To open
  them to users with access to a single database, give the instance a
  **dedicated login** whose server grants you control, or let Fleetdock create
  **per-database roles** that the engine itself confines: other databases,
  account tables, server files and global settings are then unreachable
  whatever SQL is typed. The engine-owned `mysql`/`sys`/`postgres` databases
  can only be opened by users with `instance:write` in every mode. Keep
  `instance:write` for administrators.
- **Network:** instance hosts are checked when saved and when dialled —
  loopback (in production), link-local, cloud-metadata and the metadata
  database itself are refused. Choose `tls_mode` per instance; use
  `verify-full` across untrusted networks.
- **SSH tunnels:** an external instance can be reached through an SSH bastion
  (`ssh_tunnel` on the instance; private key with optional passphrase, or
  password). The network policy then applies to the bastion address, and the
  database host is resolved on the bastion. The bastion's host key is pinned on
  first use (TOFU) and shown as a SHA256 fingerprint; a different key is
  refused until the pin is reset (`reset_ssh_host_key`). Adding, moving or
  removing the tunnel requires re-entering the admin password. Backups and
  restores reach the database through a short-lived local port forward.
  Public endpoints (gateway) are not available for tunnelled instances.
- **Sessions:** the dashboard's session is an `HttpOnly`, `SameSite=Strict`
  cookie (`fleetdock_session`), so script on the page cannot read it.
  State-changing requests authenticated by the cookie must send
  `X-Fleetdock-CSRF: 1`; bearer-token API clients are unaffected. A
  Content-Security-Policy confines the dashboard to its own origin, and signing
  out revokes all of the user's sessions server-side.
- **Agent credentials:** a server's agent token can be revoked (the agent is
  cut off at once) or rotated from the server's page. Rotating issues a
  single-use, 24-hour install command bound to that server; running it
  re-enrolls the same server with a fresh token. The agent runs as root, as
  Docker provisioning requires, inside a sandboxed systemd unit.
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
