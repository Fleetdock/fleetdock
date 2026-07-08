# db-manager — Roadmap

Analysis of what is **built**, what is **scaffolded but not wired up**, and what
should be implemented next. Grounded in the current code (not aspirational).

> **Status:** Phases 1 (automation layer), 2 (observability & dashboard) and 3
> (core database operations) are **implemented** — scheduled backups +
> retention, audit log, notifications & alerts, overview dashboard, metrics
> history, **instance provisioning (plain Docker, no Swarm/Traefik), move
> database, restore verification, and PostgreSQL/MySQL engines**. Phases 4–6
> (live-admin depth, security/multi-tenancy, quality/DX) remain.

---

## TL;DR — the biggest gap

The **database schema already defines a whole automation + observability layer
that has no code behind it.** Migration `0001_init.sql` creates these tables, but
nothing in Go reads or writes them:

| Table | Status | What's missing |
|-------|--------|----------------|
| `backup_schedules` | table only | cron scheduler, CRUD API, UI |
| `backups.expires_at` / `retention_days` | column only | retention/pruning worker |
| `audit_log` (+ immutability trigger) | table only | write path, read API, UI |
| `notification_channels` | table only | channel CRUD, dispatch, UI |
| `alert_rules` | table only | evaluation engine, UI |
| `outbox` | table only | transactional outbox publisher |
| `server_health` | single-row upsert | history/time-series + charts; `active_connections` column is never populated |

Closing this gap is the highest-leverage work because the data model is already
designed for it — it's mostly application code, not migrations.

---

## Phase 1 — Finish the automation layer (schema already exists) — ✅ DONE

### 1.1 Scheduled backups ⭐ (top priority) — ✅ DONE
- **Backend:** cron/interval parser; a scheduler loop in [worker.go](backend/internal/worker/worker.go)
  that enqueues `backup` jobs from due `backup_schedules` rows (reuse the existing
  jobs engine); mark resulting backups `type = scheduled`.
- **API:** `GET/POST/PATCH/DELETE /v1/backup-schedules` (follow the destinations handler shape).
- **Frontend:** a Schedules page/section (cron picker, target instance/db, destination, retention).
- Only the domain type in [backup.go](backend/internal/domain/backup/backup.go) mentions `scheduled` today — no scheduler, handler, or repo exists.

### 1.2 Retention & pruning — ✅ DONE
- Worker pass that deletes backups past `expires_at` (both the S3 object via the
  storage layer and the metadata row). Set `expires_at = created_at + retention_days`
  on backup creation.

### 1.3 Audit log — ✅ DONE
- The table and its immutability trigger exist; there is **no write path**.
- Add audit writes on every mutating action (login, user/role changes, instance/db
  create/drop, backup/restore, grants). Cleanest via middleware around mutating routes
  in [router.go](backend/internal/interfaces/httpapi/router.go), or a service-level hook.
- Add `GET /v1/audit` (filter by actor/resource/date — indexes already exist) + a read-only UI page.

### 1.4 Notifications & alerts — ✅ DONE
- `notification_channels` + `alert_rules` are empty scaffolds.
- Build channel CRUD (email / Slack / generic webhook), a dispatcher, and alert
  evaluation (e.g. backup failed, server offline, disk > threshold).
- Wire the unused **`outbox`** table as the transactional outbox so events are
  delivered reliably (write event in the same tx as the state change; a publisher drains it).

---

## Phase 2 — Observability & dashboard — ✅ DONE

### 2.1 Overview dashboard — ✅ DONE
- Added `/dashboard` landing page (fleet health, instance/db counts, backup and
  operation status, automation summary), backed by `GET /v1/overview`. Root and
  login now redirect there.

### 2.2 Metrics history — ✅ DONE
- Added `server_health_history` time-series (one row per heartbeat) with
  CPU/memory/disk/connection charts on the server detail page
  (`GET /v1/servers/{id}/metrics`), plus retention pruning. The heartbeat now
  collects and stores `active_connections`.

---

## Phase 3 — Core database operations — ✅ DONE

### 3.1 One-click "move database" (composite op) — ✅ DONE
- A `db_moves` saga orchestrates `backup → restore → verify → optional drop of
  source`, driven by operation-completion hooks. Start it from a database's
  detail page ("Move"); track it on the **Moves** page. `GET/POST /v1/moves`.

### 3.2 Restore verification — ✅ DONE
- Restore now downloads to a temp file and **verifies the sha256 checksum
  before mutating anything**, then counts the restored tables and reports the
  count on the operation (and on the move).

### 3.3 Instance provisioning — ✅ DONE
- The agent now launches MariaDB **Docker containers** (generated root password,
  named volume, published port) via a `provision_instance` operation, plus
  `start`/`stop`/`restart`/`remove` lifecycle ops. Managed from the server detail
  page ("Provision new") and the instance detail page. `install.sh` installs
  Docker. **No Docker Swarm / Traefik** — plain per-node Docker, since databases
  are stateful and pinned to their node.

### 3.4 More engines (PostgreSQL, MySQL) — ✅ DONE
- **MySQL** reuses the MariaDB client (same wire protocol/tooling). **PostgreSQL**
  has a full `engine.Client` (pgx: ping/list/create/drop/count, `pg_dump`/`psql`
  for backup/restore). The `instances.engine` CHECK now allows
  `mariadb`/`mysql`/`postgres`; provisioning is engine-aware (image, admin user,
  data path, port); `install.sh` and the control-plane image ship both client
  tool sets.

---

## Phase 4 — Live DB administration depth

Current live admin covers users, grants, table list, and a row browser. Add:
- **SQL query console** (read-only by permission, with row/timeout limits).
- **Table schema/DDL & index viewer.**
- **Export** query/table results (CSV) via presigned upload, like backups.

---

## Phase 5 — Security & multi-tenancy

- **Audit trail** (see 1.3) — table-stakes for a control plane.
- **Per-resource authorization / ownership scoping.** RBAC is currently global
  (permission-per-action). Consider project/team scoping and per-resource grants.
- **Scoped API tokens** — tokens are "scoped like sessions" (i.e. full user perms);
  add real per-token scopes.
- **SSO / OIDC** login and optional **2FA/MFA**.
- **Encryption-key rotation** path for `MDCP_ENCRYPTION_KEY` (envelope re-wrap) — today rotating it is destructive.

---

## Phase 6 — Quality, DX & housekeeping

- **Remove committed editor swap files** — these are tracked in git by accident:
  - `backend/internal/app/backup/service.go.7754060948526263564`
  - `backend/internal/infra/postgres/job_repository.go.7631853387065953621`
  - `backend/internal/interfaces/httpapi/instance_handler.go.4284399055078841731`
  - `backend/internal/platform/engine/admin.go.1205626397609277575`

  Delete them and add `*.go.[0-9]*` (or the editor's pattern) to [.gitignore](.gitignore).
- **Test coverage** — only 3 test files exist (server service, server handler, crypto).
  The backup, database, dbadmin, operation, instance, and user services are untested;
  they own the riskiest logic (credentials, physical drops, restores).
- **API documentation** — publish an OpenAPI/Swagger spec for the `/v1` surface.
- **Frontend:** loading/empty/error states audit; a global toast/notification for
  operation completion instead of manual refresh.

---

## Suggested order

1. **Scheduled backups + retention** (1.1, 1.2) — most-requested, schema-ready.
2. **Audit log** (1.3) — needed before this is trusted in production.
3. **Overview dashboard + metrics history** (2.1, 2.2) — makes the product feel complete.
4. **Move-database composite op** (3.1) — the headline "Vercel for DBs" feature.
5. **Notifications/alerts** (1.4), then **more engines** (3.4).
6. Housekeeping (Phase 6) can be done in parallel at any time — start by deleting the stray `.go.<n>` files.
