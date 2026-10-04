# Fleetdock — Roadmap

What ships in each release and what we are building next. For release mechanics see
[RELEASING.md](RELEASING.md).

> **Unreleased (next release)** — production hardening and a full database
> manager: least-privilege console roles, SSRF guard, TLS per instance, data and
> structure editing, monitoring and health probe, multi-statement console with
> history, backup download/verify, SQL import. See [CHANGELOG.md](CHANGELOG.md).
>
> **v0.4.0 – v0.7.1** — one image / one domain / one command, macOS install,
> system databases, PostgreSQL schema browsing.
>
> **v0.3.0** (2026-07-12) — rebrand to Fleetdock, auth/SSRF hardening, release
> automation polish. Audit log removed to simplify scope.
>
> **v0.1.0** (2026-07-10) — initial open-source release: control plane, agent,
> backups, RBAC, notifications, live DB admin, production docs, GHCR images.
> Backend test coverage ~**9%** (strong in a few services, thin on worker/repos).

---

## Shipped in v0.3.0

- **Rebrand to Fleetdock** — `FLEETDOCK_*` env prefix, GHCR image names, Go module path, API token prefixes
- **Release automation** — [RELEASING.md](RELEASING.md), `docker-compose.ghcr.yml`, Dependabot, CI coverage artifact
- **Auth hardening** — JWT token epoch invalidates browser sessions on password change/reset; proxy-aware login rate limiting (`FLEETDOCK_TRUST_PROXY_HEADERS`)
- **SSRF-resistant webhooks** — notification webhook/Slack delivery blocks loopback, link-local, and cloud metadata targets
- **MariaDB admin** — password escaping fix for special characters
- **Audit log removed** — table, permissions, and UI dropped ([migration](backend/internal/infra/postgres/migrations/0009_drop_audit.sql))
- Docs: [ROADMAP.md](ROADMAP.md) backlog, README GHCR quick-start, [docs/screenshots/README.md](docs/screenshots/README.md)

---

## Shipped in v0.1.x

- Fleet control plane (servers, managed/external instances, databases)
- Agent install (`install.sh`), Docker provisioning, move/restore sagas
- Backups to S3/R2, schedules, retention, destinations
- RBAC, scoped grants, scoped API tokens, encryption-key rotation
- Notifications/alerts, overview + server metrics
- Live admin: users, grants, tables, SQL console, schema viewer, CSV export
- OpenAPI + `/docs`, Docker Compose, CI, smoke tests
- Production guides: [DEPLOYMENT](docs/DEPLOYMENT.md), [OPERATIONS](docs/OPERATIONS.md), [SECURITY-CHECKLIST](docs/SECURITY-CHECKLIST.md)
- Automated releases → GHCR + GitHub Releases ([RELEASING.md](RELEASING.md))

---

## Now — launch polish

| Task | Status | Notes |
|------|--------|-------|
| Real README screenshots | todo | Replace placeholder; see [docs/screenshots/README.md](docs/screenshots/README.md) |
| GHCR package public | todo | Maintainer: GitHub Packages → `fleetdock` → change visibility |
| One-command install in README | done | `curl -sSL https://fleetdock.dev/install.sh \| sh` |
| Publish install.sh to fleetdock.dev | todo | `cd fleetdock-web && npm run sync:install`, then deploy |
| End-to-end install.sh verification | todo | Fresh Linux VM with real DNS: certificate issuance, agent enrolment, re-run upgrade path |
| `fleetdock reset-admin-password` | done | `api reset-password` subcommand |

---

## Testing & CI

| Task | Status | Notes |
|------|--------|-------|
| CI coverage artifact | done | Uploaded from `ci.yml` |
| Dependabot (Go, npm, Actions) | done | `.github/dependabot.yml` |
| Worker integration tests | todo | Scheduled backups, offline detection, outbox |
| Backup/restore completion hooks | todo | Happy path + failure paths |
| Postgres repo integration tests | partial | Engine integration tests + migration idempotency in CI; repository tests still todo |
| Frontend unit tests | partial | vitest for SQL classification/format; component tests todo |
| Playwright E2E smoke | todo | Login → list servers |
| Coverage gate (e.g. 25%) | todo | After critical paths covered |
| Compose boot smoke test | done | CI `docker` job: readiness, login, CSP |

---

## Security & enterprise

| Task | Status | Notes |
|------|--------|-------|
| JWT session invalidation on password change | done | Token epoch in v0.3.0 |
| SSRF protection for outbound webhooks | done | `netsafe` client in v0.3.0 |
| Proxy-aware login rate limiting | done | `FLEETDOCK_TRUST_PROXY_HEADERS` in v0.3.0 |
| httpOnly cookie sessions (replace localStorage JWT) | todo | Reduces XSS blast radius |
| SSO / OIDC | todo | Enterprise login |
| 2FA / MFA | todo | Admin accounts |
| Cloudflare Tunnel / TCP proxy for external DBs | todo | No open DB ports from internet |
| SQL console write-gating review | done | Per-database least-privilege roles; confirmation before writes |

---

## Operations & deployment

| Task | Status | Notes |
|------|--------|-------|
| Helm chart / K8s manifests | todo | |
| Prometheus `/metrics` for control plane | todo | Complement `/healthz` / `/readyz` |
| Connection pooling to instances | todo | Each admin request opens its own connection today |
| Managed-instance stats via the agent | todo | The probe reaches managed instances only when their port is reachable from the control plane |
| Staging upgrade dry-run documented | todo | v0.3.0 → v0.3.1 rehearsal |
| Multi-replica API guidance | done | Advisory-locked housekeeping and migrations; see OPERATIONS |
| Terraform module (single VM) | todo | Optional |
| Changelog automation | todo | release-please or CI guard; see discussion in CONTRIBUTING |

---

## Product

| Task | Status | Notes |
|------|--------|-------|
| Audit log export (CSV/JSON) | cancelled | Audit log removed in v0.3.0 |
| Replica / HA topology modeling | todo | |
| More notification channels | todo | PagerDuty, Discord, … |
| Point-in-time recovery | todo | Binlog / WAL archiving |
| Engine config tuning & version upgrades | todo | my.cnf / postgresql.conf, in-place upgrade |
| Engine parity (MySQL/Postgres vs MariaDB admin) | done | Postgres live admin shipped |
| Environment profile templates | todo | staging vs prod `.env` examples |

---

## Community

| Task | Status | Notes |
|------|--------|-------|
| RELEASING.md | done | |
| Demo video / GIF | todo | README + release page |
| “Good first issue” labels | todo | Maintainer |
| Comparison doc (vs phpMyAdmin, cloud consoles) | todo | |
| Public demo instance (read-only) | todo | |

---

## Suggested priority order

1. Real screenshots + public GHCR packages
2. Worker + backup integration tests (raise coverage on money paths)
3. Postgres integration tests in CI
4. httpOnly sessions or SSO (pick based on user feedback)
5. Cloudflare Tunnel for external instances
6. Helm chart + Prometheus metrics

Contributions welcome — pick any **todo** row and open a PR. See [CONTRIBUTING.md](CONTRIBUTING.md).
