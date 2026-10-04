# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

- **Least-privilege console roles.** The SQL console, table browser, CSV
  export, data editing and structure editing no longer run as the instance's
  admin (root) account. Each database gets a Fleetdock-managed read-only role
  (`fleetdock_ro_<id>`) and read-write role (`fleetdock_rw_<id>`), created on
  first use and confined by the engine to that one database: other databases,
  `mysql.user` / `pg_authid`, server files (`LOAD_FILE`, `INTO OUTFILE`,
  `pg_read_file`), account management and global settings are out of reach.
  PostgreSQL read roles default to read-only transactions. Roles are dropped
  with their database. Verified by integration tests against MariaDB 11.4,
  MySQL 8.4 and PostgreSQL 16.
- **System databases** (`mysql`, `sys`, `postgres`) can only be opened in the
  console/browser by users with `instance:write`.
- **SSRF guard for database hosts.** Instance hosts (and addresses reported by
  agents) are checked at save time and again at dial time: loopback,
  link-local, cloud-metadata addresses and the control plane's own metadata
  database are refused. Loopback is allowed in development
  (`FLEETDOCK_ALLOW_LOOPBACK_DB_HOSTS`). Connection errors are reported
  generically so instance forms cannot be used as a port scanner.
- **Changing an instance's host now requires re-entering its password**, so a
  stored admin password is never sent to a new host.
- **TLS for instance connections**: per-instance `tls_mode` (`disable`,
  `prefer` — the default, `require`, `verify-full`). MySQL/MariaDB connections
  previously used no TLS at all.
- PostgreSQL passwords are sent as **SCRAM-SHA-256 verifiers**, never as
  plaintext that could reach server logs.
- **Server-side statement timeouts** on interactive sessions
  (`max_statement_time` / `max_execution_time` / `statement_timeout`), so an
  abandoned query is killed on the server, plus a result-size cap.
- Locked or migrating databases refuse writes from the console and editors.
- `EXPLAIN ANALYZE` / `ANALYZE` are classified as writes.
- Content-Security-Policy on the dashboard and API, HSTS in the bundled Caddy.
- `POST /v1/auth/logout` ends every browser session of the user (token epoch);
  the dashboard calls it on sign-out.
- Login takes the same time for unknown and known emails.
- Production refuses JWT secrets / encryption keys shorter than 16 characters
  (and warns below 32).
- SQL file imports run as the database's read-write role, never as root.
- New installs generate a random password for the bundled metadata Postgres.
- Dependencies: Next.js 16.3.8 (critical advisory), Go 1.26 toolchain
  (Go 1.25 is end-of-life), `golang.org/x/crypto`, `klauspost/compress`.

### Added

- **Data Browser** (`/data`, `g t`). A workspace for working with data like
  a desktop database manager: pick a database, find its tables and views in
  a searchable sidebar (grouped by schema), and open each one in its own tab.
  A single click previews a table, a double click keeps it open; tabs can be
  reordered, closed in bulk and are restored on the next visit with their
  filters, sort, page and column widths. The grid has sticky headers,
  resizable columns, multi-column sort, keyboard navigation, copying as
  TSV/JSON/INSERT, a value panel for long and JSON values, and one-click jumps
  along foreign keys. Edits are made in place — change cells, add, duplicate
  and delete rows — and stay pending, highlighted, until saved with
  Ctrl+S; the SQL can be reviewed first, and rows that fail stay on screen
  with the reason. SQL console tabs and the structure editor live in the
  same workspace, and functions, procedures, triggers, sequences and events
  are listed under "Routines & more" with their definitions.

- **Automatic discovery.** Every minute Fleetdock checks each database server
  it can log in to and keeps its database list in sync: new databases appear
  on their own (labelled `discovered`), databases dropped outside Fleetdock
  are marked **not found on server** after about three minutes (and raise a
  `database.missing` notification), and come back to active when they
  reappear. Nothing is ever deleted. Database servers on a machine that
  Fleetdock can't reach directly ask that machine's agent instead (at most
  hourly). `POST /v1/instances/{id}/probe` ("Check now") checks one server
  immediately. Migration `0019` adds the `missing` status, `last_seen_at`
  and `missing_since`.
- **Overview that says what needs attention**: offline servers, unreachable
  database servers, failed backups and backup checks, scheduled databases
  with no backup in 7 days, missing databases and failed tasks — each
  linking to the problem, filtered by what the user may see. A **Get
  started** checklist guides new installs (connect a database server, add
  backup storage, schedule backups, set up alerts). `GET /v1/overview`
  gains `setup` and `attention`.
- **Command palette** (Ctrl/⌘ K or `/`): jump to any page, server, database
  server or database. Shortcuts: `g` + letter for sections, `?` for help.
- Running-task indicator in the top bar; task toasts name what they ran on
  ("Backup · orders finished") with a link.
- **Backups tab on every database**: its schedule, next backup, history and
  "Back up now". The Backups page searches by database or server name.
- Operations, backups and schedules in the API carry display names
  (`resource_name`, `database_name`, `instance_name`); `GET /v1/backups`
  takes `search`.
- End-to-end browser tests (`frontend/e2e`, `npm run e2e`) against a running
  install, including a discovery round trip against a real PostgreSQL.
- `docs/glossary.md`: the words the dashboard uses, and what the code calls
  them.

- **Data editing**: filtered/sorted/searchable data browser with stable
  ordering; insert, edit and delete rows by primary key (or NOT NULL unique
  key) — each change must hit exactly one row or nothing is written; CSV
  import (all or nothing).
- **Structure management** without SQL: create table, add/modify/rename/drop
  columns (keeping existing defaults), foreign keys, indexes, rename, truncate
  and drop (type-to-confirm); lists of views, functions, procedures, triggers,
  sequences and events; PostgreSQL DDL now includes constraints, defaults and
  identity.
- **Monitoring**: live sessions with cancel / terminate, server status
  counters and configuration per instance.
- **Instance health probe**: every minute the worker checks each instance and
  records health, version and latency, and fills database size and connection
  counts (previously never populated).
- **SQL console**: multi-statement scripts on one session (quotes, comments,
  dollar quotes and `DELIMITER` understood), confirmation before anything
  writes, cancel, per-user history and saved queries.
- **Backups**: download (presigned link), delete, and verification by test
  restore into a scratch database, with the result shown per backup.
- **SQL file import** (`.sql` / `.sql.gz`, up to 2 GB).
- Set a database account's password from the instance page.
- `PATCH /v1/instances/{id}`: edit name, host, port, TLS mode and credentials.
- CLI: `fleetdock backup-db`, `fleetdock reset-admin-password`; `fleetdock
  update` takes a metadata backup first and refuses to continue without one.
- `api reset-password <email>` subcommand (used by the CLI).
- CI: engine integration tests, migration idempotency, a compose boot smoke
  test and frontend unit tests; releases run the test suite first.

### Changed

- **The database page no longer has Tables and SQL console tabs.** Browsing,
  editing and querying moved to the Data Browser; the page keeps its
  overview, connectivity, credentials, backups and access, plus a Data card
  that opens the Data Browser or a new SQL query there. Old links
  (`?tab=tables`, `?tab=query`, `?table=…`) redirect to the Data Browser.

- **Dashboard redesign.** Seven sections instead of twelve flat pages:
  Overview, Servers, Databases, Backups (history, schedules, storage),
  Activity, Access (users, roles, API tokens) and Settings (notifications,
  profile). Old addresses redirect permanently (`/operations` → `/activity`,
  `/destinations` → `/backups/storage`, `/users` → `/access/users`, …).
- **Databases and database servers in one place**: databases are grouped by
  server, with a flat "All databases" view and search. One **Connect a
  database server** wizard replaces the separate "Add instance" forms
  (create a new one, use one on my server, or connect to one anywhere) and
  shows the databases it finds.
- **"Test connection" and "Import DBs" are gone from the dashboard** —
  discovery does both on its own. The `test-connection` and
  `import-databases` endpoints still work and are marked deprecated.
- Works on phones and tablets: the sidebar becomes a drawer (or an icon rail
  on mid-size screens), tables scroll inside their card, forms stack.
- Plain language throughout (database server, backup storage, activity,
  "Check backup", encryption…), relative times with the exact time on hover,
  readable cron schedules ("Every day at 02:00 UTC"), and help hints on
  technical fields.
- Every destructive action asks in an accessible dialog (no more browser
  `confirm()` / `alert()`), errors and results appear as toasts, dialogs
  trap focus and close with Esc, and form labels are linked to their fields.
- A user granted access to single databases sees the plain database list
  instead of an error.

- **`POST /v1/databases/{id}/query` returns `{results: [...], error?}`** (one
  result per statement) instead of a single result object.
- The database detail page is split into components; the "Move" action is now
  "Copy / move" and also covers renaming.
- With several API replicas, housekeeping (schedules, retention, alerts,
  probes) runs on one replica at a time (advisory lock), and migrations
  serialise.
- Docker image has a `HEALTHCHECK`.

### Fixed

- PostgreSQL values in the table browser, SQL console and CSV export are
  shown as PostgreSQL prints them (`13.37`, `2026-01-02`, `{"a": 1}`, a
  UUID's canonical form) instead of as decoded driver values such as
  `{1337 -2 false finite true}` or a byte array.
- Choosing "Custom…" in the backup schedule form did nothing.
- The connect wizard lost its result screen when the first database server
  was added.
- Searches treated `%` and `_` in the search text as wildcards.

- Backups of PostgreSQL and MySQL databases were recorded as `mariadb-dump`.
- Restoring a PostgreSQL ≤ 16 database from a dump made by pg_dump 17 (the
  image's client) failed on `SET transaction_timeout`; that line is now
  dropped during restore.
- "Test" on a backup destination reported success for a bucket that does not
  exist.
- Failed dump/restore operations showed a client warning instead of the error.
- PostgreSQL restores could report success after a partial failure
  (`ON_ERROR_STOP`, single transaction).
- CSV exports were cut off after 30 seconds.
- Operations orphaned by a crash or restart stayed `running` forever; they are
  now failed through the normal completion path.
- A panic in a background job could take the API down.
- The `fleetdock` CLI and `install.sh` used GNU-only `sed -i` (broken on
  macOS) and could corrupt values containing `|` or `&`; re-running the
  installer no longer resets a pinned release tag to `latest`.
- Instances that were deleted still showed through their databases.
- Deleting a provisioned instance's data volume, or dropping a database,
  now requires typing its name; failed background actions show a toast.

## [0.4.0 – 0.7.1]

These versions were tagged without their own changelog sections; their
changes are collected here.

### Added

- **System databases are now discovered.** Import registers PostgreSQL's
  `postgres` maintenance database and MySQL/MariaDB's `mysql` and `sys` schemas
  alongside user databases, so they can be browsed and backed up. They are
  flagged `system` in the API and cannot be dropped or un-registered — the
  control plane connects to them for every admin operation. The purely virtual
  `information_schema` and `performance_schema` are still skipped, as they
  cannot be dumped.
- **One-command install:** `curl -sSL https://fleetdock.dev/install.sh | sh`.
  Installs Docker if missing, generates secrets, starts the stack behind Caddy
  with automatic HTTPS, and prints the dashboard URL and bootstrap password.
  Without `--domain` it uses an `<ip>.sslip.io` name so TLS works with no DNS
  setup.
- **`fleetdock` CLI** for day-2 operations: `status`, `logs`, `update`,
  `domain`, `gateway enable|disable`, `config`, `psql`, `doctor`,
  `backup-config`, `uninstall`.
- **Caddy is bundled** as a compose service on 80/443 with automatic Let's
  Encrypt. TLS is no longer a manual step.
- Published images are now **multi-arch** (`linux/amd64`, `linux/arm64`).
  Previous releases were amd64-only and could not run on ARM hosts.
- [docs/MIGRATION-single-image.md](docs/MIGRATION-single-image.md).

### Changed

- **One image instead of two.** `ghcr.io/fleetdock/fleetdock` contains both the
  control plane and the dashboard. The Go binary is the only listener: it serves
  `/v1`, `/agent`, `/healthz`, `/readyz`, `/docs`, `/openapi.yaml` and
  `/install.sh` in-process and reverse-proxies everything else to a supervised
  Next.js process on loopback.
- **One domain instead of up to three.** `FLEETDOCK_DOMAIN` drives
  `FLEETDOCK_PUBLIC_URL`, `FLEETDOCK_CORS_ORIGIN` and
  `FLEETDOCK_GATEWAY_PUBLIC_HOST`. The dashboard and API share an origin; the
  gateway reuses the same hostname because it speaks raw TCP on its own ports.
- **The dashboard no longer bakes in an API URL.** `NEXT_PUBLIC_API_URL` is
  optional and only for split-origin deployments. Published images previously
  hardcoded `https://fleetdock.dev`, which made them unusable by anyone else.
- **One compose file.** `docker-compose.yml` is production-shaped, has no bind
  mounts, and is complete on its own; `docker-compose.build.yml` builds from
  source. Requires Docker Compose 2.23+.
- **External database access is opt-in.** The gateway sits behind a `gateway`
  compose profile, so a default install publishes 80/443 instead of 51 ports.
- The metadata `postgres` service can be scaled to zero for hosted PostgreSQL.

### Fixed

- **PostgreSQL credentials now reach every schema.** Access profiles, custom
  grants and revokes were hardcoded to `public`, so a credential issued against
  a database whose tables live in another schema was silently powerless — and a
  revoke left privileges behind if any had been granted elsewhere by hand. All
  three paths now enumerate the database's schemas. Two PostgreSQL limits
  remain: the grant is a snapshot (a schema created later is not covered), and
  `ALTER DEFAULT PRIVILEGES` only affects objects created by the granting role.
- **Table browsing distinguishes schemas.** `ListTables` returns the schema
  alongside the name, and browse/export accept `schema.table`. Previously
  `public.orders` and `sales.orders` were both shown as `orders` and opening
  either one resolved to whichever the catalogue returned first. Grant listings
  report per-schema privileges instead of `public` only.
- `/_next/static/*` assets are no longer served with `Cache-Control: no-store`.
  The API's no-store now applies only to API responses.
- Wrong-method API calls keep returning `405` rather than falling through to the
  dashboard's HTML 404.
- Release images are tagged both `v<version>` and `<version>`; the documented
  `FLEETDOCK_RELEASE_TAG=v0.1.0` previously matched nothing in GHCR.
- Added a repository `.dockerignore`; build contexts no longer include
  `frontend/node_modules` and `.next`.

### Removed

- `ghcr.io/fleetdock/fleetdock-backend` and `-frontend` are no longer published.
- `docker-compose.prod.yml` and `docker-compose.ghcr.yml`.

## [0.3.0] - 2026-07-12

### Added

- [RELEASING.md](RELEASING.md) — tag, GHCR, and smoke-test runbook for maintainers
- `docker-compose.ghcr.yml` — run published images without local builds
- Dependabot for Go, npm, and GitHub Actions
- CI uploads backend `coverage.out` artifact with summary in logs
- [docs/screenshots/README.md](docs/screenshots/README.md) — how to capture real dashboard screenshots

### Changed

- **Rebrand to Fleetdock** — product rename; website [fleetdock.dev](https://fleetdock.dev)
- Environment prefix `FLEETDOCK_*`
- Agent binary/service paths, API token prefixes (`fleetd_`, `fleetr_`, `fleeta_`)
- Go module `github.com/Fleetdock/fleetdock/backend`
- GHCR images `fleetdock-backend` / `fleetdock-frontend`
- [ROADMAP.md](ROADMAP.md) rewritten with post-v0.1.0 backlog (removed completed phase sections)
- README: GHCR quick-start, removed AI-generated screenshot placeholder
- Login rate limiting derives client IP from the transport peer by default; set `FLEETDOCK_TRUST_PROXY_HEADERS=true` when the API runs behind a trusted reverse proxy that sets `X-Forwarded-For`

### Security

- JWT sessions carry a per-user token epoch; password change or reset bumps the epoch and invalidates outstanding browser sessions (API tokens are unaffected)
- Notification webhook and Slack delivery use SSRF-resistant outbound HTTP — loopback, link-local, and cloud metadata addresses are blocked
- MariaDB user password handling in DB admin escapes special characters correctly

## [0.1.0] - 2026-07-10

Initial open-source release.

### Added

- Self-hosted control plane for MariaDB, MySQL, and PostgreSQL
- Server agent with one-command install (`install.sh`)
- Instance provisioning via Docker on connected servers
- Database lifecycle: create, drop, lock/unlock, import, move
- Backup and restore with S3-compatible destinations and scheduled backups
- Retention pruning for expired backups
- RBAC with JWT authentication and API tokens
- Audit log for mutating actions
- Notification channels (email, Slack, webhook) and alert rules
- Overview dashboard with fleet health and metrics history
- Live DB administration: users, grants, table browser, SQL console
- Next.js dashboard with TanStack Query
- Docker Compose stack with local Postgres for development
- OpenAPI specification for the `/v1` API surface
- CI pipeline (Go tests, lint, frontend build)
- Production guides: [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md), [docs/OPERATIONS.md](docs/OPERATIONS.md), [docs/SECURITY-CHECKLIST.md](docs/SECURITY-CHECKLIST.md)
- `docker-compose.prod.yml` overlay and `deploy/Caddyfile.example` for TLS reverse proxy
- GitHub Actions release workflow publishing GHCR images and release archives on version tags
- HTTP smoke tests for health, OpenAPI, install script, and login flow
- Config validation tests for production secret enforcement
- README hero screenshot and production deployment section

### Fixed

- `/readyz` is now a public readiness probe (no authentication required)

[Unreleased]: https://github.com/Fleetdock/fleetdock/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/Fleetdock/fleetdock/releases/tag/v0.3.0
[0.1.0]: https://github.com/Fleetdock/fleetdock/releases/tag/v0.1.0
