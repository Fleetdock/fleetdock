# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **Rebrand to Fleetdock** — product rename; website [fleetdock.dev](https://fleetdock.dev)
- Environment prefix `FLEETDOCK_*`
- Agent binary/service paths, API token prefixes (`fleetd_`, `fleetr_`, `fleeta_`)
- Go module `github.com/TajBrains/fleetdock/backend`
- GHCR images `fleetdock-backend` / `fleetdock-frontend`

### Added

- [RELEASING.md](RELEASING.md) — tag, GHCR, and smoke-test runbook for maintainers
- `docker-compose.ghcr.yml` — run published images without local builds
- Dependabot for Go, npm, and GitHub Actions
- CI uploads backend `coverage.out` artifact with summary in logs
- [docs/screenshots/README.md](docs/screenshots/README.md) — how to capture real dashboard screenshots

### Changed

- [ROADMAP.md](ROADMAP.md) rewritten with post-v0.1.0 backlog (removed completed phase sections)
- README: GHCR quick-start, removed AI-generated screenshot placeholder

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

[Unreleased]: https://github.com/TajBrains/fleetdock/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/TajBrains/fleetdock/releases/tag/v0.1.0
