# db-manager — Roadmap

Analysis of what is **built**, what is **scaffolded but not wired up**, and what
should be implemented next. Grounded in the current code (not aspirational).

> **Status:** Phases 1–6 are **implemented**. Phase 1 (automation layer),
> 2 (observability & dashboard), 3 (core database operations), 4 (live-admin
> depth: SQL console, schema/DDL viewer, CSV export), 5 (security &
> multi-tenancy: scoped RBAC, scoped API tokens, encryption-key rotation) and
> 6 (quality/DX: OpenAPI, tests, toasts). The only deferred Phase-5 item is
> **SSO/OIDC login and optional 2FA/MFA**.

---

## TL;DR — what's next

The control plane is feature-complete for an initial open-source release. The
highest-leverage remaining work is:

| Area | Status | What's missing |
|------|--------|----------------|
| Per-resource RBAC / multi-tenancy | done | scoped grants enforced on resources + filtered lists |
| Per-token scopes | done | tokens carry validated scopes + optional expiry |
| Encryption key rotation | done | `make rotate-keys` re-wraps under a new key id |
| OpenAPI | done | embedded spec served at `/openapi.yaml` + Redoc at `/docs` |
| SSO / OIDC + 2FA/MFA | not started | enterprise login (deferred) |
| Test coverage | improving | expand to backup/restore hooks, worker, repos |

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

## Phase 4 — Live DB administration depth — ✅ DONE

Live admin covers users, grants, table list, row browser, a SQL query console
with write gating, a **table schema/DDL & index viewer**, and **CSV export** of
query/table results.

---

## Phase 5 — Security & multi-tenancy — ✅ DONE (except SSO/OIDC)

- **Per-resource authorization / ownership scoping.** ✅ Role grants are scoped
  `global` / `server` / `database` (`user_roles.scope_type/scope_id`); the
  request principal carries scoped grants and every resource route resolves the
  resource's ancestry (database → instance → server) before authorizing. List
  endpoints are filtered to the caller's readable scope. Manage grants per user
  on the **Users** page (`GET/POST /v1/users/{id}/role-grants`,
  `DELETE …/{grantId}`).
- **Scoped API tokens** ✅ — a token restricts to its declared scopes (empty =
  inherit the user's grants); scopes are validated against the catalog and the
  creator's permissions, with an optional expiry.
- **Encryption-key rotation** ✅ — envelope re-wrap via a multi-key keyring
  (`MDCP_ENCRYPTION_KEY_ID` + `MDCP_ENCRYPTION_KEYS_OLD`) and an offline
  `make rotate-keys` (`cmd/rotate-keys`); payload ciphertext is never touched.
- **SSO / OIDC** login and optional **2FA/MFA** — deferred.

---

## Phase 6 — Quality, DX & housekeeping — ✅ DONE

- **Test coverage** ✅ — service/handler/middleware tests including the authz
  domain, scoped Principal, token scope validation, keyring rotation, and
  resource-scoped enforcement. (Backup/restore hooks and repos can grow further.)
- **API documentation** ✅ — hand-authored OpenAPI at
  [`backend/internal/openapi/openapi.yaml`](backend/internal/openapi/openapi.yaml),
  embedded and served at `GET /openapi.yaml` with a Redoc page at `GET /docs`; a
  unit test enforces drift against `router.go`.
- **Frontend** ✅ — a global toast system fires on operation completion (no more
  manual refresh); loading/empty/error states use shared `ui.tsx` primitives.

---

## Suggested next steps

1. **SSO/OIDC + 2FA/MFA** (the deferred Phase-5 item) — enterprise login.
2. **Expand test coverage** — backup/restore completion hooks, worker, repos.
