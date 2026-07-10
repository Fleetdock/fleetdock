# db-manager — Roadmap

What ships in **v0.1.0** and what we are building next. For release mechanics see
[RELEASING.md](RELEASING.md).

> **v0.1.0** (2026-07-10) — initial open-source release: control plane, agent,
> backups, RBAC, audit log, notifications, live DB admin, production docs, GHCR
> images. Backend test coverage ~**9%** (strong in a few services, thin on worker/repos).

---

## Shipped in v0.1.x

- Fleet control plane (servers, managed/external instances, databases)
- Agent install (`install.sh`), Docker provisioning, move/restore sagas
- Backups to S3/R2, schedules, retention, destinations
- RBAC, scoped grants, scoped API tokens, encryption-key rotation
- Audit log, notifications/alerts, overview + server metrics
- Live admin: users, grants, tables, SQL console, schema viewer, CSV export
- OpenAPI + `/docs`, Docker Compose, CI, smoke tests
- Production guides: [DEPLOYMENT](docs/DEPLOYMENT.md), [OPERATIONS](docs/OPERATIONS.md), [SECURITY-CHECKLIST](docs/SECURITY-CHECKLIST.md)
- Automated releases → GHCR + GitHub Releases ([RELEASING.md](RELEASING.md))

---

## Now — launch polish

| Task | Status | Notes |
|------|--------|-------|
| Real README screenshots | todo | Replace placeholder; see [docs/screenshots/README.md](docs/screenshots/README.md) |
| GHCR packages public | todo | Maintainer: GitHub Packages → change visibility |
| `NEXT_PUBLIC_API_URL` repo variable | todo | Set before release if not localhost; see RELEASING.md |
| GHCR quick-start in README | done | `docker-compose.ghcr.yml` |
| End-to-end install.sh verification | todo | Fresh VM + document friction |

---

## Testing & CI

| Task | Status | Notes |
|------|--------|-------|
| CI coverage artifact | done | Uploaded from `ci.yml` |
| Dependabot (Go, npm, Actions) | done | `.github/dependabot.yml` |
| Worker integration tests | todo | Scheduled backups, offline detection, outbox |
| Backup/restore completion hooks | todo | Happy path + failure paths |
| Postgres repo integration tests | todo | testcontainers or CI service container |
| Frontend unit tests | todo | Login shell, nav, critical forms |
| Playwright E2E smoke | todo | Login → list servers |
| Coverage gate (e.g. 25%) | todo | After critical paths covered |

---

## Security & enterprise

| Task | Status | Notes |
|------|--------|-------|
| httpOnly cookie sessions (replace localStorage JWT) | todo | Reduces XSS blast radius |
| SSO / OIDC | todo | Enterprise login |
| 2FA / MFA | todo | Admin accounts |
| Cloudflare Tunnel / TCP proxy for external DBs | todo | No open DB ports from internet |
| SQL console write-gating audit | todo | Security review |

---

## Operations & deployment

| Task | Status | Notes |
|------|--------|-------|
| Helm chart / K8s manifests | todo | |
| Prometheus `/metrics` for control plane | todo | Complement `/healthz` / `/readyz` |
| Staging upgrade dry-run documented | todo | v0.1.0 → v0.1.1 rehearsal |
| Multi-replica API guidance | todo | Worker is singleton today |
| Terraform module (single VM) | todo | Optional |

---

## Product

| Task | Status | Notes |
|------|--------|-------|
| Audit log export (CSV/JSON) | todo | |
| Replica / HA topology modeling | todo | |
| More notification channels | todo | PagerDuty, Discord, … |
| Engine parity (MySQL/Postgres vs MariaDB admin) | todo | |
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
