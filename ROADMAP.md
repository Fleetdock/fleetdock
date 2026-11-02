# Fleetdock — Roadmap

What has shipped and what comes next. For what each release changed see
[CHANGELOG.md](CHANGELOG.md); for how releases are made and what 1.x keeps
stable see [RELEASING.md](RELEASING.md).

---

## 1.0 — what is in it

**Fleet.** Servers with a sandboxed agent (one-command install, in-place
updates, version checks), database servers that Fleetdock creates in Docker or
that you connect anywhere (directly, through an SSH bastion, or through the
optional gateway), automatic database discovery and health checks, MySQL,
MariaDB and PostgreSQL in one dashboard.

**Backups.** Schedules with retention to your own S3-compatible storage,
automatic test restores, restore and copy/move between servers of the same
engine, SQL import, opening a backup next to the live database to compare.

**Working with data.** The data browser (filters, inline editing with undo,
saved views, PostgreSQL schema choice, full screen), the SQL console with
least-privilege roles, the ER diagram, schema history and drift with DDL to
align, fleet queries across up to 500 databases, index and query insights.

**Access and audit.** Roles with server- and database-scoped grants, scoped
API tokens with expiry, an audit log of every change and refused request,
per-account sign-in lockout.

**Operations.** One image and one command to install, `fleetdock` CLI
(update, backups, key rotation, diagnostics), checksummed migrations with
upgrade checks, Prometheus metrics, signed releases, CI that upgrades the last
release, dumps every engine with the image's tools and runs end-to-end tests.

---

## Next — 1.1

| Item | Notes |
|------|-------|
| Two-factor sign-in (TOTP) | Recovery codes; owners and admins can be required to use it |
| SSO / OIDC | Sign in with your identity provider; map groups to roles |
| Session list | See and end your signed-in browsers one by one |

## Later

| Area | Item |
|------|------|
| Backups | Point-in-time recovery (binlog / WAL archiving) |
| Backups | Ship metadata backups to backup storage from the dashboard |
| Fleet | Instance statistics through the agent, so managed instances need no port open to the control plane |
| Fleet | Replica / HA topology |
| Fleet | Engine configuration (`my.cnf` / `postgresql.conf`) and in-place engine upgrades |
| Fleet | `GET /v1/engines`: supported engines and versions from the server |
| Access | Cloudflare Tunnel / TCP proxy for external databases without open ports |
| Operations | Helm chart; Terraform module for a single VM |
| Operations | OpenTelemetry traces |
| Operations | Configurable worker intervals |
| Notifications | More channels (PagerDuty, Discord, Microsoft Teams) |
| Dashboard | Translations |
| Community | Public read-only demo, comparison with phpMyAdmin and cloud consoles |

Contributions welcome: open an issue to discuss an item before a large PR. See
[CONTRIBUTING.md](CONTRIBUTING.md).
