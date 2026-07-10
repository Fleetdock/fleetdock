# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Main branch ruleset definition (`.github/rulesets/protect-main.json`) and `scripts/setup-branch-protection.sh`
- [docs/BRANCH_PROTECTION.md](docs/BRANCH_PROTECTION.md) — apply via script or GitHub UI

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

[Unreleased]: https://github.com/TajBrains/db-manager/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/TajBrains/db-manager/releases/tag/v0.1.0
