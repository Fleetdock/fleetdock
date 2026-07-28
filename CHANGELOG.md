# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
