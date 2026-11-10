# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Page transitions.** Pages fade in with a slight rise when you move
  between them. Settings → Appearance → Animations chooses Full, Reduced (a
  plain fade) or Off (no page or interface animations); the system's
  reduce-motion setting always turns them off.
- **Appearance settings.** Settings → Appearance chooses the theme (Light,
  Dark or System), the style (Classic, or a compact gray Neutral style) and
  a Borderless option. Saved in the browser and applied before the page
  shows.

### Changed

- **New logo.** The dashboard's sidebar, sign-in page, browser tab and
  home-screen icons use the new Fleetdock logo, in a light and a dark
  version. The full logo kit is in `brand/`.

### Fixed

- **The page no longer jumps sideways when a dialog opens.** The space for
  the page scrollbar is always reserved.
- **Data browser: the Structure view scrolls as one page.**
- **Adding or editing a column offers every column type.** Only
  `varchar(255)` was listed, because the type box only suggested types
  matching what it already held. It now lists the database engine's types
  (MySQL/MariaDB or PostgreSQL), grouped by kind, with Custom… for any
  other.
- **The dashboard works over plain HTTP on a LAN address.** Running a query
  or saving row edits failed there, because the browser only offers
  `crypto.randomUUID()` on HTTPS or localhost.

## [0.10.0] - 2026-11-07

Upgrading: see
[OPERATIONS.md — Upgrading to 0.10](docs/OPERATIONS.md#upgrading-to-010).

### Added

- **Fleetdock knows its version.** Release builds stamp the version and
  commit into the control plane and the agent. The dashboard shows it at
  the bottom of the sidebar, `fleetdock version` and `/api version` print
  it, `GET /v1/version` reports it with the oldest supported agent, the
  served OpenAPI document carries it, and agents report their real version
  instead of `0.1.0`.
- **Agent updates.** A server whose agent is older than the control plane
  shows **Update** in the server list, an *Agent update available* notice
  with the command on its page, and an item on the overview (*Agent too
  old* when it is below the supported minimum). Running the installer
  without a registration token updates an enrolled agent in place, keeping
  its identity. API: servers report `agent_status` (`current`, `outdated`,
  `unsupported`, `unknown`).
- **Upgrade checks.** CI upgrades an install of the latest release to the
  new code and checks its data survives.
- **PostgreSQL 18 and MariaDB 11.8** can be created as new database servers
  (PostgreSQL 17 is now the default). MySQL 8.0, past its end of life, is no
  longer offered for new servers; existing ones keep working.
- **Prometheus metrics.** `GET /metrics` reports the build, HTTP requests
  and latency, finished operations by type and result, the worker's last
  run, and fleet gauges (servers, database servers, databases, backups and
  failures in the last 24 hours, running operations, the last backup).
  Enabled by `FLEETDOCK_METRICS_TOKEN`, which the scraper sends as a bearer
  token. `/readyz` now also fails when the background worker has stopped.
- **Administrators see everyone's API tokens.** *Access → API tokens* lists
  other users' working tokens (owner, scopes, expiry, last use) for anyone
  who manages users, and revokes them. API: `GET /v1/api-tokens`,
  `DELETE /v1/api-tokens/{id}`.
- **Metadata backups you can schedule.** `fleetdock backup-db --keep N`
  keeps the newest N; the docs give a nightly cron line that also copies
  them off the host.
- **Signed releases.** Images and the checksum files are signed with
  cosign (keyless, from the release workflow); RELEASING.md shows how to
  verify them.
- **Back where you were after signing in again.** When a session ends, the
  sign-in page says so and returns to the page you were on; the SQL
  console keeps the script you were writing (per database, in this
  browser). Unknown addresses outside the dashboard get a proper "Page not
  found", and a failure of the whole page an error page instead of a blank
  one.

### Changed

- **Dependencies.** Next.js 16.4.5, pgx 5.12; the dashboard is built and
  tested with Node 24 LTS, the same major as the image's runtime.
- **Compatibility rules.** [RELEASING.md](RELEASING.md#compatibility)
  states what releases keep compatible (the `/v1` API, agents,
  configuration, upgrades) and how things are deprecated before they go. The OpenAPI document now describes every
  operation's error response (`{error: {code, message, field}}`), the
  cookie session next to bearer tokens, and the response bodies of servers,
  database servers, databases, backups, operations, users, roles, tokens,
  backup storage, schedules, notification channels and alert rules; a test
  keeps those in step with what the handlers return.
- **Key rotation in one command.** `fleetdock rotate-keys` replaces
  `FLEETDOCK_ENCRYPTION_KEY` and re-encrypts every stored credential, with
  backups first and the old key put back if anything fails. The image now
  includes it as `/api rotate-keys` (the separate `cmd/rotate-keys` program
  is gone), and the compose file passes `FLEETDOCK_ENCRYPTION_KEY_ID` and
  `FLEETDOCK_ENCRYPTION_KEYS_OLD` to the container.
- **Safer upgrades.** Fleetdock records the SHA-256 of every migration and
  refuses to start on a metadata database migrated by a newer version, or
  whose applied migrations have changed, instead of running against a
  schema it does not match. With `FLEETDOCK_RUN_MIGRATIONS=false` it also
  refuses to start while migrations are pending.
- **The image ships the PostgreSQL 18 client** (on Alpine 3.23, with
  Node.js 24), so backups of PostgreSQL 18 servers work: a newer `pg_dump`
  reads older servers, never the reverse. CI now dumps and restores MySQL
  8.4, MariaDB 11.4 and PostgreSQL 16 and 18 with the image's own tools.
- **CI before every release**: the release workflow runs the whole CI suite
  (integration, upgrade, end-to-end and image-scan jobs included) before it
  publishes. CI gained the Playwright end-to-end tests against a booted
  stack, `govulncheck`, `npm audit` for runtime dependencies, a Trivy scan
  of the image, Dependabot for base images, and a floor under backend test
  coverage.
- Release candidates (`v0.10.0-rc.1`) no longer move the `latest` image tag
  and are published as pre-releases. Releases also ship an arm64 API
  binary.

### Removed

- **Deprecated API routes.** `POST /v1/instances/{id}/test-connection` and
  `POST /v1/instances/{id}/import-databases` are gone; use
  `POST /v1/instances/{id}/probe`, which checks the connection and refreshes
  the database list (databases are also discovered every minute).
- **`mariadb_version` in API responses.** Servers and database servers
  report `engine_version` only; registering a database server takes
  `engine_version` (the `mariadb_version` alias is no longer read). Agents
  may still send it.

### Security

- **Password guessing against one account is stopped.** After 10 failed
  sign-ins to an account within 15 minutes, it refuses sign-in for the rest
  of those minutes, from any address (the per-address limit still applies
  too). Refusals are in the audit log.
- **New API tokens expire after 90 days by default** (7 days to 1 year, or
  never, with a warning).
- **Notification secrets are encrypted.** Webhook and Slack URLs and the
  SMTP password of notification channels are kept in the encrypted secret
  store like other credentials, instead of in plain text in the channel's
  settings. Channels saved by earlier versions are encrypted on the first
  start.
- **Backup storage endpoints follow the database host rules**: an S3
  endpoint on a link-local or cloud-metadata address, the metadata database,
  or loopback in production is refused, at once and with the reason.
- **Client addresses behind a proxy can no longer be chosen by the client.**
  With `FLEETDOCK_TRUST_PROXY_HEADERS`, Fleetdock takes the last
  `X-Forwarded-For` entry (the one the proxy adds) instead of the first.
  Behind nginx with `$proxy_add_x_forwarded_for`, the first could be set by
  the client to dodge the login limit and appear as anyone in the audit
  log. The nginx example now sets `$remote_addr`.
- Agent enrolment (`POST /agent/v1/register`) is rate limited like sign-in.
- The API docs page loads a pinned Redoc version checked by its integrity
  hash, instead of whatever `latest` served.
- **The agent installer checks what it downloads**: the binary must match
  the SHA-256 the control plane publishes
  (`GET /agent/v1/binary/{os}/{arch}/sha256`). It is written beside the
  old one and renamed into place, so an update no longer fails on a
  running binary.

### Fixed

- **Passwords longer than 72 bytes failed with a server error.** bcrypt
  refuses them; they are now rejected with a clear message (72 plain
  letters, fewer with accents or non-Latin letters).
- **Backups and restores of MySQL 8 servers failed from the image**: the
  MariaDB client could not log in with `caching_sha2_password`, MySQL 8's
  default, because Alpine ships its authentication plugins in a separate
  package. The image (and the agent installer on Alpine) now installs
  `mariadb-connector-c`. Found by the new dump test in CI.
- **Creating a PostgreSQL 18 server failed**: the official image keeps its
  data under `/var/lib/postgresql` from 18 on and refuses the old volume
  path.
- **Email alerts could gain headers**: a title carrying a name with a line
  break (PostgreSQL allows one in a database name) could add headers such
  as `Bcc:`. Header values are now kept on one line.
- **A backup could be restored to a database server of another engine**,
  failing only when the restore ran. Restoring, copying or moving now
  requires the same engine family (PostgreSQL, or MySQL/MariaDB), and the
  dialogs only offer such servers.

## [0.9.3] - 2026-11-02

### Added

- **Choose PostgreSQL schemas.** In databases with several schemas, the
  data browser's table tree and the ER diagram show the schemas you pick
  (a picker above the table search and in the diagram's toolbar: tick
  schemas, **Only**, or **All schemas**), remembered per database and
  shared between the two. A database opens on `public`, or its biggest
  schema. Searching the tree still finds tables in the other schemas
  ("N more in other schemas — Show"). In the diagram, a foreign key into a
  schema that is not shown draws that table as a dashed card with
  **Show schema**. API: `GET /v1/databases/{id}/diagram?schemas=a,b`; the
  response lists every schema with its counts.
- **Saved table views.** Save a table's filters, search, sort, hidden and
  frozen columns, widths and page size as a named view (the **Views** menu
  in the data browser's toolbar), private or shared with everyone who can
  open the database. Only its author changes a view; someone with
  `database:write` may delete a shared one. The menu marks a tab that has
  drifted from its view and offers **Update**; **Copy link** opens the table
  in that view (`/data?db=…&table=…&saved=<id>`). API:
  `/v1/databases/{id}/table-views`; changes are audited as `table_view.*`.

## [0.9.2] - 2026-11-01

### Added

- **Data browser full screen.** A button at the end of the tab bar, or
  Alt+Shift+F, hides Fleetdock's sidebar and top bar; remembered per
  browser. In full screen the bar also offers the browser's own full
  screen and the theme switch.
- **Columns, frozen columns, a row panel, taller rows.** Choose which
  columns the grid shows (Columns menu, or Hide column in a header's menu)
  and freeze the first ones while scrolling sideways; both stay with the
  tab. The row panel (Shift+Enter) shows the selected row as a form, every
  column in full and editable. More → Taller rows switches the density.
- **More filter conditions, from any column.** *is one of*, *is none of*,
  *between*, *ends with* and *doesn't contain* join the conditions; a
  column header's menu starts a filter on it, and clicking a filter chip
  edits it. API: `in`, `not_in`, `between`, `ends_with`, `not_contains`,
  with `values[]` for lists and ranges.
- **Export what you see.** Export CSV in the data browser downloads the
  rows its filters, search and sort select (all rows without filters).
  API: `POST /v1/databases/{id}/tables/{table}/export` with a browse
  request.
- **Keyboard.** The `?` help lists the data browser's shortcuts;
  Alt+PageDown / Alt+PageUp turn the page of rows.

### Changed

- **The ER diagram has its own page** (`/databases/{id}/diagram`) that
  uses the whole window, opened from the database page's **Diagram** button
  or the data browser's table list.
- **The database page is organised in tabs**: Overview (details, how to
  connect, last backup and schedule, index health), Connection (private and
  public access, app credentials), Backups, Access and Schema. Tabs that
  need the server login are hidden without one; Copy or move moved into the
  ⋯ menu. `?tab=diagram` links open the diagram page.
- **SQL console.** The editor and the results split the tab, with a
  separator to drag. Results use the data browser's grid (keyboard, copy as
  TSV/JSON/INSERT, resizable columns, sorting). Choose 200, 1,000 or 5,000
  rows per statement; the API now caps the console at 5,000 rows instead of
  falling back to 100 above 1,000.
- Pages of more than 150 rows draw only the rows in view, so 500-row pages
  scroll smoothly; the first load of a table shows the grid's outline.

### Fixed

- A right-click that scrolled a half-visible column or cell into view
  closed the menu it had just opened.
- Numbers in foreign-key columns no longer hide under the follow button.

## [0.9.1] - 2026-11-01

### Added

- **ER diagram.** A database's **Diagram** tab draws its tables and
  views with their columns, primary keys and row estimates, and every
  foreign key between them, laid out automatically. Click a table to
  highlight its relations, double-click it (or its name) to open it in the
  data browser, find a table by name, show only a table and its neighbours
  (1–3 steps), narrow a PostgreSQL database to one schema, and download the
  diagram as PNG or SVG. Positions you drag are kept in the browser.
  Databases with more than 150 tables open on their most connected table.
  API: `GET /v1/databases/{id}/diagram`; the column catalog
  (`GET /v1/databases/{id}/columns`) now also reports `nullable` and
  `primary_key`.
- **Audit log.** *Activity → Audit log* shows who did what: sign-ins and
  failed sign-ins, users, roles, grants and API tokens, servers, database
  servers and databases, backups, backup storage and schedules,
  notification settings, console scripts that write (with the script),
  data-browser edits and undo, structure changes, exports and dumps. Every
  event carries the user — or the API token, or the CLI — with IP address
  and client, the resource and the outcome; requests refused for lack of
  permission are kept as *denied*. Filter by period, kind of action, user
  and result, open an event for its details, export CSV, or open a
  database's own events from its menu. Owners and admins can read it
  (`audit:read`). Events are kept for `FLEETDOCK_AUDIT_RETENTION` (365
  days; `0` keeps them). `fleetdock reset-admin-password` is recorded too.
  API: `GET /v1/audit-events`.

### Fixed

- **Deleting a user who appears in the audit trail failed** with "table
  audit_events is append-only": unlinking their events on delete counted
  as changing them. Their events now stay, with their email.

## [0.9.0] - 2026-11-01

### Security

- **Next.js 16.4.3.** The dashboard was on Next.js 16.4.0, affected by
  GHSA-2464-fhr4-w9xw (unauthenticated remote code execution in the image
  optimizer with SVG files), GHSA-673q-mjm6-r5xr and GHSA-p3m5-59mp-jv5r.
- **Go 1.26.10.** Builds use Go 1.26.10 and golang.org/x/net 0.62.0, which fix
  standard-library vulnerabilities in `net/http`, `crypto/tls`, `crypto/x509`
  and `mime/multipart`.
- **Production refuses the demo stack's secrets.** `FLEETDOCK_ENV=production`
  now also rejects secrets marked `not-for-production` and the demo admin
  password, so a copy of the demo compose file cannot run with published
  keys.

### Added

- **Fleet query: one script, many databases.** The new *Fleet query* page
  runs a SQL script on up to 500 databases in the background, a few at a
  time, and merges the results into one table (with CSV export). Access is
  checked per database when the run starts and again before each one runs;
  scripts that write need a typed confirmation, run on one database first
  and stop at the first failure by default. Runs can be canceled and
  retried. `FLEETDOCK_FLEET_MAX_PARALLEL` (16) caps the work per process,
  `FLEETDOCK_FLEET_RUN_RETENTION` (30 days) how long runs are kept. API:
  `/v1/fleet-runs`.
- **Schema history and drift.** Turn on *Track changes* on a database's new
  **Schema** tab and every structural change — from Fleetdock, a migration
  tool or someone's psql — is recorded as a snapshot with what changed, and
  announced as a `database.schema_changed` notification. *Compare with…*
  diffs two databases on the same engine and writes the DDL that aligns them
  (destructive steps commented out for review); a snapshot can be compared
  with the live schema to go back to it. Checks run every
  `FLEETDOCK_SCHEMA_SNAPSHOT_INTERVAL` (15m). API:
  `/v1/databases/{id}/schema/snapshots`, `/v1/databases/{id}/schema/watch`,
  `POST /v1/schema/compare`.
- **Compare a table with a backup, restore single rows.** *Compare with a
  backup…* in the data browser restores a backup into a temporary `fds_…`
  database on the same server, lists the rows changed, deleted or added since
  (by primary key, in pages), and puts the picked rows back as they were.
  Restores are journaled, so **Undo** reverts them. The copy is dropped after
  `FLEETDOCK_BACKUP_COPY_TTL` (2h; extendable to 24h);
  `FLEETDOCK_BACKUP_COPY_MAX_BYTES` caps the backup size. API:
  `/v1/databases/{id}/backup-copies` (create, list, extend, discard, diff,
  restore-rows).
- **Preview writes in the SQL console.** A script with writes can be dry-run
  first: it runs in one transaction that is always rolled back
  (`POST /v1/databases/{id}/query` with `preview: true`), reporting rows
  affected per statement and, for single-table UPDATE / DELETE, a sample of
  up to 20 rows before and after; reads later in the script see the
  preview's changes. Only statements a rollback fully undoes are allowed,
  autocommit is off for the whole run on MySQL/MariaDB, and databases with
  non-transactional tables are refused. Previews are not kept in the
  history.
- **Undo for the data browser.** Every insert, edit and delete made in the
  data browser is journaled with the row before and after, captured in the
  same transaction. Saving shows **Undo**; a table's **Change history** lists
  the edits of the last week (`FLEETDOCK_ROW_CHANGE_RETENTION`) with undo per
  row or per save. An undo first checks that the row still looks as the
  edit left it and asks before overwriting a later change. Binary values,
  identity and generated columns and the session time zone round-trip
  exactly (`GET /v1/databases/{id}/changes`,
  `POST /v1/databases/{id}/changes/{changeId}/undo`,
  `POST /v1/databases/{id}/changes/undo`). Row edits now return the
  journal entry, and generated columns are read-only in the grid.
- **SQL autocompletion from the real schema.** The console completes the
  database's tables, schemas and columns (with their types) and the
  engine's keywords (`GET /v1/databases/{id}/columns`).
- **Foreign keys both ways.** Composite foreign keys now link to their
  referenced row, and a row's context menu opens the rows of other tables
  that reference it (`GET /v1/databases/{id}/tables/{table}/referenced-by`).
- **Find a table or column anywhere with Ctrl/Cmd+K.** A background loop
  keeps an index of every reachable database's table and column names
  (`FLEETDOCK_CATALOG_REFRESH_INTERVAL`, 30 minutes, and right after DDL
  made through Fleetdock); `GET /v1/search/schema` returns only databases
  the caller could open in the data browser.
- **Explain and Analyze in the console.** A statement's plan is drawn as a
  tree with expected and actual rows, time or cost per step, and warnings
  for full scans, filesorts, temporary tables, disk spills and stale
  statistics (`POST /v1/databases/{id}/explain`). Analyze runs reads only,
  in a read-only transaction that is rolled back.
- **Index health.** A database's page and each table's structure flag
  indexes never used since statistics started and indexes made redundant
  by another one, with the `DROP INDEX` statement to review
  (`GET /v1/databases/{id}/index-insights`).
- **Top queries.** An instance's monitoring lists its most expensive
  statements by total time, time per run or frequency, from
  `pg_stat_statements` or `performance_schema`, and explains how to turn
  them on when they are off (`GET /v1/instances/{id}/top-queries`).
- **Demo stack.** `deploy/demo` starts a throwaway install with sample
  MariaDB, MySQL and PostgreSQL databases bound to 127.0.0.1;
  `scripts/screenshots.sh` builds it, seeds it and captures the screenshots
  used in the README and on fleetdock.dev.

### Changed

- **A refreshed dashboard.** Every page is rebuilt on one design system
  (shared tokens, controls, tables and charts) with the Geist typeface,
  bundled with the image. Charts have time axes, tabs work with the arrow
  keys, toasts wait while you hover or focus them, and pages show loading,
  error and not-found states of their own.

### Fixed

- **Databases with `-` in their names are managed.** Applications that name
  databases after UUIDs (`tenant_ffc07774-1db5-…`) had those databases
  skipped by discovery without a warning, and refused by grants, the data
  browser and backups. Database names may now contain `-` anywhere but the
  first character. Charset, collation and role names keep the stricter rule.
  Thanks to @shahawiz (#67).
- The SQL console now honours a `DELIMITER` line that follows a comment.
- **Backup verification no longer shows up as a database.** The temporary
  `fdv_…` database a test restore uses was picked up by discovery while it
  existed and then reported as missing once dropped. Names `fdv_`/`fds_`
  followed by 16 hex digits are now reserved for Fleetdock's temporary
  restores and never registered; ones registered before are removed from
  the inventory.
- **Long statements on MySQL and MariaDB no longer fail after 30 seconds.**
  The driver's per-read timeout cut off any statement that returned nothing
  for 30 seconds (a slow `UPDATE`, an `ALTER` on a big table) with "invalid
  connection", whatever its own limit. It now outlasts the statement's
  timeout.
- **MySQL and MariaDB through an SSH tunnel.** Connections failed with
  "invalid connection" because the tunnel rejected I/O deadlines. Deadlines
  are now emulated, so a bastion that stops answering still times out
  instead of blocking.
- **Database discovery is no longer cancelled by slow size queries.** The
  probe saves an instance's database list before collecting sizes, each with
  its own deadline, so instances with many tables (or behind an SSH tunnel)
  have their databases discovered.

## [0.8.1 – 0.8.5]

These versions were tagged without their own changelog sections; their
changes are collected here.

### Security

- **The dashboard session is an httpOnly cookie.** Login sets a
  `fleetdock_session` cookie (`HttpOnly`, `SameSite=Strict`, `Secure` over
  HTTPS) instead of the dashboard keeping the JWT in `localStorage`, so
  injected script can no longer read the session. Cookie-authenticated
  requests that change state must send `X-Fleetdock-CSRF: 1`. API clients keep
  using `Authorization: Bearer`; the login response still carries the token.
  Signing out clears the cookie, and a token left in `localStorage` by an
  earlier release is removed.
- **Revoke and rotate a server's agent token.** A server's page has *Rotate
  agent token* and *Revoke agent token*
  (`POST /v1/servers/{id}/agent-token/rotate`,
  `DELETE /v1/servers/{id}/agent-token`). Revoking cuts the agent off at once;
  rotating also returns a single-use install command that re-enrolls the same
  server with a fresh token. Unused install commands are listed on the Servers
  page and can be revoked.
- **The agent's systemd unit is sandboxed.** The agent still runs as root
  (Docker provisioning needs it), now with `NoNewPrivileges`, `ProtectSystem`,
  `ProtectHome`, `PrivateTmp`, kernel and control-group protections and a
  `0077` umask. Re-run the install command on a server to pick up the new unit.

### Changed

- **Database page redesign.** Headline figures (size, tables, views, open
  connections) across the top; connection details, public access,
  application credentials and backups/access in the main column; details and
  quick links alongside. Connection details include a copyable,
  credential-free connection URL, and gateway warnings are shown as callouts
  with plain-language TLS status.

- **Choose the login used for data browsing.** The data browser, SQL
  console, exports and SQL imports used to connect as roles Fleetdock created
  on the database server without asking (`fleetdock_ro_*` /
  `fleetdock_rw_*`). Each instance now has a **Data browsing connects as**
  setting: its admin login (the default, limited to instance administrators),
  a dedicated login you provide, or the Fleetdock-managed roles. Instances
  that already have managed roles keep using them.

### Fixed

- **"The database rejected the login" in the data browser** when a
  Fleetdock-managed role was dropped or its password changed outside
  Fleetdock: the role is now repaired and the request retried, and a server
  that still refuses it gets an error naming the role.

- **Installer on macOS and WSL refuses `sudo`.** Docker Desktop runs as the
  logged-in user and a local install lives in their home folder; under
  `sudo` it went to `/opt/fleetdock` as root, next to the user's own
  `~/.fleetdock`, and the two fought over the same containers.
- **A fresh install no longer starts over an earlier install's data.** When
  Docker still holds Fleetdock's database volume but no `.env` was found, the
  installer stops and says where the old configuration is (or how to discard
  the data), instead of generating new keys that cannot open it — which left
  the control plane crash-looping.
- **An existing install is upgraded where it is.** The installer looks for
  the caller's own install first (`~/.fleetdock`, or `/opt/fleetdock` for
  root) and says so when the one it finds belongs to another user.
- **`--port` works on upgrades**, moving an existing local install to the new
  port; a local install from before local mode is made private to the computer
  (127.0.0.1) as a new one would be.
- **A busy port is no longer a dead end.** Run in a terminal, a local install
  offers the next free port instead of stopping.
- The installer warns about an older `fleetdock` command from an earlier
  install that would run instead of the new one, and shows the control
  plane's log when the stack fails to start.

### Added

- **Download a backup without backup storage.** "Back up now" on a database
  offers **Download to this computer**: the control plane runs the same dump
  as a stored backup and streams it to the browser as `<database>-<time>.sql.gz`.
  Nothing is kept in Fleetdock and no bucket is needed
  (`GET /v1/databases/{id}/dump`, checked first with
  `POST /v1/databases/{id}/dump/check`; needs `backup:write`). Restore such a
  file with Import SQL.
- **"Check now" on a database server's page** moved to the header, next to
  Edit.
- **Connect external databases through an SSH tunnel.** An external database
  server can be reached through an SSH bastion, signing in with a private key
  (optionally passphrase-protected) or a password. The SSH secret is stored
  encrypted; the bastion's host key is pinned on the first connection and any
  later change is refused until it is reset from the database server's page.
  Live administration, monitoring, discovery, backups and restores all go
  through the tunnel. Public endpoints are not available for these servers.
- `fleetdock port [<n>]` shows or changes a local install's dashboard port.
  `fleetdock config get` says when a setting is not set instead of printing
  nothing.

## [0.8.0] - 2026-10-06

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

- **Local install — "only this computer".** `install.sh --local` (automatic on
  macOS and WSL2, offered as a choice elsewhere) installs for one user with no
  root, serves `http://localhost`, and publishes the dashboard on 127.0.0.1
  only (`FLEETDOCK_HTTP_BIND` / `FLEETDOCK_HTTPS_BIND`). `--port` picks another
  port when 80 is taken. Windows is supported through WSL2 with Docker
  Desktop; Git Bash/MSYS get directions instead of "unsupported platform". See
  [docs/LOCAL.md](docs/LOCAL.md).
- **Guided installer.** Run in a terminal, `install.sh` asks where it is
  installing, for the domain and the admin email, shows a summary and waits
  for confirmation; it ends with numbered next steps. `--yes` (or no terminal,
  as under CI and cloud-init) asks nothing, as before.
- `host.docker.internal` reaches the machine running Fleetdock from the control
  plane on Linux too; the Host field says to use it instead of `localhost`.

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

- **Docs reorganised around what you want to do.** The README is now a short
  quick start plus a "Which guide do I need?" table. The feature list moved to
  [docs/FEATURES.md](docs/FEATURES.md), the environment variable reference to
  [docs/CONFIGURATION.md](docs/CONFIGURATION.md), and running from source to
  [CONTRIBUTING.md](CONTRIBUTING.md).
- In a local install, `fleetdock domain` defaults to `http://`, and switches
  between loopback-only and all-interfaces listening with the name.

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

- **Metadata database password after upgrading.** Installs made with an
  installer that generated `FLEETDOCK_POSTGRES_PASSWORD` before
  `docker-compose.yml` read it have a database created with the old fixed
  password; once compose used the generated one, the control plane could not
  log in. `install.sh`, `fleetdock start` and `fleetdock update` now set the
  database role's password to the `.env` value before starting.
- The installer and `fleetdock delete` no longer exit silently when there is no
  terminal: the `/dev/tty` check redirected a special builtin, which is fatal
  in POSIX `sh`.

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

[Unreleased]: https://github.com/Fleetdock/fleetdock/compare/v0.10.0...HEAD
[0.10.0]: https://github.com/Fleetdock/fleetdock/compare/v0.9.3...v0.10.0
[0.9.3]: https://github.com/Fleetdock/fleetdock/compare/v0.9.2...v0.9.3
[0.9.2]: https://github.com/Fleetdock/fleetdock/compare/v0.9.1...v0.9.2
[0.9.1]: https://github.com/Fleetdock/fleetdock/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/Fleetdock/fleetdock/compare/v0.8.5...v0.9.0
[0.8.0]: https://github.com/Fleetdock/fleetdock/releases/tag/v0.8.0
[0.3.0]: https://github.com/Fleetdock/fleetdock/releases/tag/v0.3.0
[0.1.0]: https://github.com/Fleetdock/fleetdock/releases/tag/v0.1.0
