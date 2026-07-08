# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-07-08

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

[Unreleased]: https://github.com/TajBrains/db-manager/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/TajBrains/db-manager/releases/tag/v0.1.0
