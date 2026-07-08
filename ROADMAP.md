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

## TL;DR — what's next

The control plane is feature-complete for an initial open-source release. The
highest-leverage remaining work is:

| Area | Status | What's missing |
|------|--------|----------------|
| SQL console depth | partial | read-only/write gating exists; richer schema/DDL viewer |
| SSO / OIDC | not started | enterprise login |
| Per-token scopes | partial | API tokens intersect with user perms but aren't independently scoped |
| Encryption key rotation | not started | rotating `MDCP_ENCRYPTION_KEY` is destructive today |
| Test coverage | improving | service tests for auth, backup, database, operation, agent; expand further |
| OpenAPI | done | [`docs/openapi.yaml`](docs/openapi.yaml) with CI drift check |

---

## Phase 1 — Finish the automation layer (schema already exists) — ✅ DONE

Scheduled backups, retention pruning, audit log, notifications/alerts, and the
transactional outbox are all implemented. See [README.md](README.md) for details.

---

## Phase 2 — Observability & dashboard — ✅ DONE

Overview dashboard, metrics history, and server health charts are implemented.

---

## Phase 3 — Core database operations — ✅ DONE

Move database saga, restore verification, instance provisioning, and
PostgreSQL/MySQL engines are implemented.

---

## Phase 4 — Live DB administration depth

Current live admin covers users, grants, table list, row browser, and a SQL
query console with write gating. Add:

- **Table schema/DDL & index viewer** (deeper than current schema endpoint).
- **Export** query/table results (CSV) via presigned upload, like backups.

---

## Phase 5 — Security & multi-tenancy

- **Per-resource authorization / ownership scoping.** RBAC is currently global
  (permission-per-action). Consider project/team scoping and per-resource grants.
- **Scoped API tokens** — tokens intersect with user permissions; add real
  per-token scopes independent of the user's full role.
- **SSO / OIDC** login and optional **2FA/MFA**.
- **Encryption-key rotation** path for `MDCP_ENCRYPTION_KEY` (envelope re-wrap) — today rotating it is destructive.

---

## Phase 6 — Quality, DX & housekeeping

- **Test coverage** — 10+ backend test files covering core services and HTTP
  middleware; expand to backup/restore completion hooks, worker, and repos.
- **API documentation** — OpenAPI spec published at [`docs/openapi.yaml`](docs/openapi.yaml); CI drift check against `router.go`.
- **Frontend:** loading/empty/error states audit; a global toast/notification for
  operation completion instead of manual refresh.

---

## Suggested order

1. **SQL console depth + schema viewer** (Phase 4) — most visible UX gap.
2. **SSO/OIDC** (Phase 5) — needed for enterprise adoption.
3. **Encryption key rotation** (Phase 5) — needed for long-running deployments.
4. **Expand test coverage** (Phase 6) — ongoing alongside feature work.
