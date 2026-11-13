# Configuration

Everything is configured through `FLEETDOCK_*` environment variables. An install
made with `install.sh` keeps them in `.env` in the install directory
(`/opt/fleetdock`, or `~/.fleetdock` for a local install) — read and change them
with the `fleetdock` command rather than by hand:

```bash
fleetdock env                     # show everything, secrets redacted
fleetdock config get FLEETDOCK_PUBLIC_URL
fleetdock config set FLEETDOCK_SMTP_HOST smtp.example.com   # writes .env and restarts
```

## What install.sh writes

These describe the install itself. `install.sh` derives them from your answers
(or flags), and `fleetdock domain` keeps the address ones in step afterwards.

| Variable | Example | What it is |
| --- | --- | --- |
| `FLEETDOCK_MODE` | `server` / `local` | `local` is an "only this computer" install: no root needed for `fleetdock`, no DNS or certificate checks in `fleetdock doctor` |
| `FLEETDOCK_DOMAIN` | `db.example.com` | the hostname the dashboard is served on |
| `FLEETDOCK_SITE_ADDRESS` | `db.example.com` / `http://localhost` | Caddy's site address: a bare name gets an automatic certificate, `http://…` serves plain HTTP |
| `FLEETDOCK_PUBLIC_URL` | `https://db.example.com` | the URL agents and the browser use; baked into the agent install command |
| `FLEETDOCK_CORS_ORIGIN` | same as the public URL | unused on a same-origin install |
| `FLEETDOCK_HTTP_BIND` | `127.0.0.1:80` | where port 80 is published. Unset = every interface. A local install sets `127.0.0.1:<port>` so only this computer can reach the dashboard |
| `FLEETDOCK_HTTPS_BIND` | `127.0.0.1:` | the same for 443. A local install binds it to a random loopback port, since nothing uses it |
| `FLEETDOCK_POSTGRES_PASSWORD` | generated | password of the bundled metadata Postgres. `fleetdock start`/`update` keep the database's password in step with this value |
| `FLEETDOCK_RELEASE_TAG` | `latest` / `v0.11.0` | image tag to run; `fleetdock update --tag` changes it |
| `FLEETDOCK_GATEWAY_ENABLED` | `false` | external database access; use `fleetdock gateway enable` |

Gateway tuning is described in [external-access.md](external-access.md):
`FLEETDOCK_GATEWAY_PUBLIC_HOST`, `FLEETDOCK_GATEWAY_SOURCE_IP_MODE`,
`FLEETDOCK_GATEWAY_DIAG_PORT`, the published port range
`FLEETDOCK_GATEWAY_PORT_RANGE_START` / `_END` (`15432`–`15481`), and the
files the control plane and HAProxy share: `FLEETDOCK_GATEWAY_CONFIG_PATH`,
`FLEETDOCK_GATEWAY_MASTER_SOCKET`, `FLEETDOCK_GATEWAY_ADMIN_SOCKET` (under
`/var/lib/fleetdock/gateway`; leave them alone unless you run HAProxy
yourself).

## Control plane

| Variable                                              | Default                 | Notes                                                                                                             |
| ----------------------------------------------------- | ----------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `FLEETDOCK_DATABASE_URL`                              | —                       | required                                                                                                          |
| `FLEETDOCK_ENV`                                       | `development`           | `production` refuses to start with default secrets                                                                |
| `FLEETDOCK_HTTP_ADDR`                                 | `:8080`                 |                                                                                                                   |
| `FLEETDOCK_JWT_SECRET`                                | dev default             | set a strong secret in production (min. 16 characters, 32+ recommended)                                           |
| `FLEETDOCK_ENCRYPTION_KEY`                            | dev default             | primary key that encrypts stored credentials; rotate with `fleetdock rotate-keys` (see OPERATIONS.md)               |
| `FLEETDOCK_ENCRYPTION_KEY_ID`                         | `master-1`              | id stamped on secrets wrapped by the primary key; use a new id when rotating                                      |
| `FLEETDOCK_ENCRYPTION_KEYS_OLD`                       | —                       | retired keys still needed to decrypt during rotation, as `id=secret,id2=secret2`                                  |
| `FLEETDOCK_PUBLIC_URL`                                | `http://localhost:8080` | URL agents/installers use to reach the API                                                                        |
| `FLEETDOCK_AGENT_BIN_DIR`                             | `/opt/fleetdock/agents` | where cross-compiled agent binaries live                                                                          |
| `FLEETDOCK_WORKER_ENABLED`                            | `true`                  | in-process worker (external-instance ops, offline detection, scheduled backups, retention, alerts, notifications) |
| `FLEETDOCK_JOB_POLL_INTERVAL`                         | `3s`                    | how often queued operations are claimed; minimum `1s`                                                             |
| `FLEETDOCK_ALERT_INTERVAL`                            | `15s`                   | how often alert rules are evaluated and pending notifications delivered; minimum `5s`                             |
| `FLEETDOCK_HEALTH_CHECK_INTERVAL`                     | `1m`                    | how often instance health, database size and connection probes run; minimum `15s`                                 |
| `FLEETDOCK_METRICS_TOKEN`                             | —                       | enables `GET /metrics` (Prometheus) for scrapers sending it as a bearer token; unset, the endpoint answers 404     |
| `FLEETDOCK_HEARTBEAT_TIMEOUT`                         | `2m`                    | no heartbeat for this long ⇒ server `offline`                                                                     |
| `FLEETDOCK_METRICS_RETENTION`                         | `168h`                  | how long per-heartbeat server metrics history is kept                                                             |
| `FLEETDOCK_ROW_CHANGE_RETENTION`                      | `168h`                  | how long data-browser edits stay in the change journal and can be undone; `0` turns the journal off                |
| `FLEETDOCK_AUDIT_RETENTION`                           | `8760h`                 | how long audit log events are kept (365 days); `0` keeps them forever                                             |
| `FLEETDOCK_CATALOG_REFRESH_INTERVAL`                  | `30m`                   | how often each database's table and column names are re-read for the fleet-wide search (Ctrl/Cmd+K); `0` disables it |
| `FLEETDOCK_SCHEMA_SNAPSHOT_INTERVAL`                  | `15m`                   | how often databases with schema history turned on are checked for structural changes; `0` checks only after DDL made through Fleetdock and on demand |
| `FLEETDOCK_FLEET_MAX_PARALLEL`                        | `16`                    | how many fleet-query databases one Fleetdock process works on at once (each run also has its own limit, 1–16) |
| `FLEETDOCK_FLEET_RUN_RETENTION`                       | `720h`                  | how long finished fleet-query runs and their results are kept |
| `FLEETDOCK_BACKUP_COPY_TTL`                           | `2h`                    | how long a backup opened for **Compare with a backup** is kept next to the database before it is dropped (extendable up to 24h) |
| `FLEETDOCK_BACKUP_COPY_MAX_BYTES`                     | `10737418240`           | largest backup (compressed size) that can be opened for comparison; the copy takes disk on the database's server |
| `FLEETDOCK_SMTP_HOST`                                 | —                       | SMTP host for email notification channels (empty ⇒ email delivery disabled)                                       |
| `FLEETDOCK_SMTP_PORT`                                 | `587`                   | SMTP port                                                                                                         |
| `FLEETDOCK_SMTP_USERNAME` / `FLEETDOCK_SMTP_PASSWORD` | —                       | SMTP auth (optional)                                                                                              |
| `FLEETDOCK_SMTP_FROM`                                 | `fleetdock@localhost`   | envelope/from address for emails                                                                                  |
| `FLEETDOCK_ADMIN_EMAIL` / `FLEETDOCK_ADMIN_PASSWORD`  | —                       | first-run bootstrap only; generate with `./scripts/generate-secrets.sh`                                           |
| `FLEETDOCK_CORS_ORIGIN`                               | `http://localhost:3000` | only used for split-origin deployments; same-origin installs never hit CORS                                       |
| `FLEETDOCK_CORS_CREDENTIALS`                          | `false`                 | split-origin only: let `FLEETDOCK_CORS_ORIGIN` send the dashboard session cookie                                  |
| `FLEETDOCK_UI_DIR`                                    | —                       | the bundled dashboard. Set by the image; empty means API-only (bare binary, dev)                                  |
| `FLEETDOCK_UI_PORT`                                   | `3000`                  | loopback port the dashboard binds; never published                                                                |
| `FLEETDOCK_ALLOW_LOOPBACK_DB_HOSTS`                   | `true` in development, `false` in production | let instances point at loopback addresses (link-local, metadata and the metadata DB are always refused) |
| `FLEETDOCK_POSTGRES_PASSWORD`                         | `fleetdock`             | compose only: password of the bundled metadata Postgres; `install.sh` generates one for new installs — never change it on an existing volume |
| `FLEETDOCK_TRUST_PROXY_HEADERS`                       | `false` (`true` in compose) | take the client address from the last `X-Forwarded-For` entry (login limits, audit log). Only while :8080 is reachable solely through your proxy |
| `FLEETDOCK_RUN_MIGRATIONS`                            | `true`                  | apply migrations on start. With `false` (migrations run as a separate job) the API refuses to start while any are pending |
| `FLEETDOCK_JWT_TTL`                                   | `24h`                   | how long a dashboard sign-in lasts |
| `FLEETDOCK_REQUEST_TIMEOUT`                           | `30s`                   | the HTTP server's read and write timeout |
| `FLEETDOCK_SHUTDOWN_TIMEOUT`                          | `15s`                   | how long a stopping process lets requests in flight finish |
| `FLEETDOCK_UI_NODE_BIN`                               | `node`                  | the Node.js binary that runs the bundled dashboard |
| `FLEETDOCK_UI_STARTUP_TIMEOUT`                        | `45s`                   | how long to wait for the dashboard to start before giving up |

Duration values use Go's syntax (`90s`, `15m`, `2h`). Invalid worker interval
values prevent the control plane from starting; other invalid durations fall
back to their defaults.

## One-off commands

| Variable | Used by | What it is |
| --- | --- | --- |
| `FLEETDOCK_NEW_PASSWORD` | `api reset-password` | the password to set; empty generates one (`fleetdock reset-admin-password` handles this) |
| `FLEETDOCK_SKIP_BACKUP` | `fleetdock update` | `1` skips the metadata backup taken before upgrading |
| `FLEETDOCK_I_UNDERSTAND` | `fleetdock config set FLEETDOCK_ENCRYPTION_KEY` | `1` allows replacing the key outright, making stored credentials unreadable; use `fleetdock rotate-keys` instead |

## Agent

Written by the installer to `/etc/fleetdock-agent/agent.env` on each server.

| Variable | What it is |
| --- | --- |
| `FLEETDOCK_URL` | the control plane's public URL (`FLEETDOCK_PUBLIC_URL`) |
| `FLEETDOCK_TOKEN` | the single-use registration token, needed until the agent has enrolled |
| `FLEETDOCK_STATE_DIR` | where the agent keeps its identity (`/var/lib/fleetdock-agent`) |

## Development

| Variable | What it is |
| --- | --- |
| `FLEETDOCK_DEV_API_URL` | `next dev` only: where the dashboard's API calls are proxied (`http://127.0.0.1:8080`) |
| `NEXT_PUBLIC_API_URL` | split-origin builds only: the API's origin, inlined at build time |
