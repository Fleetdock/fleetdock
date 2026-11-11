# Fleetdock — Roadmap

What has shipped and what comes next. For what each release changed see
[CHANGELOG.md](CHANGELOG.md); for how releases are made and what stays
compatible see [RELEASING.md](RELEASING.md).

---

## 0.10 — what is in it

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

## Next — 0.11

| Item | Notes | Issue |
|------|-------|-------|
| Two-factor sign-in (TOTP) | Recovery codes; owners and admins can be required to use it | #91 |
| SSO / OIDC | Sign in with your identity provider; map groups to roles | #92 |
| Session list | See and end your signed-in browsers one by one | #93 |

## AI First

AI agents (any MCP client) and later a built-in assistant work with your fleet,
and can never read a credential or delete data: a fixed set of read-only tools,
SQL under the read-only role, row values only when allowed and masked, changes
only as proposals a person approves. Design:
[docs/proposals/ai-first.md](docs/proposals/ai-first.md); epic #113.

| Phase | Item | Issue |
|-------|------|-------|
| 1 | MCP server: read-only, metadata only | #114 |
| 2 | Read-only SQL with row data and column masking | #115 |
| 3 | Changes as proposals a person approves | #116 |
| 4 | Built-in assistant with your own model | #117 |

## Toward 1.0

1.0 is tagged when the compatibility rules in [RELEASING.md](RELEASING.md#compatibility)
have held over a few minor releases and can become a promise for the whole 1.x
line.

## Later

| Area | Item | Issue |
|------|------|-------|
| Backups | Point-in-time recovery (binlog / WAL archiving) | #94 |
| Backups | Ship metadata backups to backup storage from the dashboard | #95 |
| Fleet | Instance statistics through the agent, so managed instances need no port open to the control plane | #96 |
| Fleet | Replica / HA topology | #97 |
| Fleet | Engine configuration (`my.cnf` / `postgresql.conf`) and in-place engine upgrades | #98 |
| Fleet | `GET /v1/engines`: supported engines and versions from the server | #99 |
| Access | Cloudflare Tunnel / TCP proxy for external databases without open ports | #100 |
| Operations | Helm chart; Terraform module for a single VM | #101 |
| Operations | OpenTelemetry traces | #102 |
| Operations | Configurable worker intervals | #103 |
| Notifications | More channels: Discord, Microsoft Teams, PagerDuty | #104, #105, #106 |
| Dashboard | Translations | #107 |
| Community | Public read-only demo; comparison with phpMyAdmin and cloud consoles | #108, #109 |

Every item has an issue: comment there before starting, so we can agree on the
approach and nobody does the same work twice. All of them are listed under the
[`roadmap` label](https://github.com/Fleetdock/fleetdock/issues?q=is%3Aissue+is%3Aopen+label%3Aroadmap);
[`help wanted`](https://github.com/Fleetdock/fleetdock/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22)
and [`good first issue`](https://github.com/Fleetdock/fleetdock/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)
mark the ones that are ready for contributors. See [CONTRIBUTING.md](CONTRIBUTING.md).
