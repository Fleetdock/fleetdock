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
| `FLEETDOCK_RELEASE_TAG` | `latest` / `v0.8.0` | image tag to run; `fleetdock update --tag` changes it |
| `FLEETDOCK_GATEWAY_ENABLED` | `false` | external database access; use `fleetdock gateway enable` |

Gateway tuning (`FLEETDOCK_GATEWAY_*`) is described in
[external-access.md](external-access.md).

## Control plane

| Variable                                              | Default                 | Notes                                                                                                             |
| ----------------------------------------------------- | ----------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `FLEETDOCK_DATABASE_URL`                              | —                       | required                                                                                                          |
| `FLEETDOCK_ENV`                                       | `development`           | `production` refuses to start with default secrets                                                                |
| `FLEETDOCK_HTTP_ADDR`                                 | `:8080`                 |                                                                                                                   |
| `FLEETDOCK_JWT_SECRET`                                | dev default             | set a strong secret in production (min. 16 characters, 32+ recommended)                                           |
| `FLEETDOCK_ENCRYPTION_KEY`                            | dev default             | primary key that encrypts credentials/S3 keys at rest; rotate via `make rotate-keys` (see Security)               |
| `FLEETDOCK_ENCRYPTION_KEY_ID`                         | `master-1`              | id stamped on secrets wrapped by the primary key; use a new id when rotating                                      |
| `FLEETDOCK_ENCRYPTION_KEYS_OLD`                       | —                       | retired keys still needed to decrypt during rotation, as `id=secret,id2=secret2`                                  |
| `FLEETDOCK_PUBLIC_URL`                                | `http://localhost:8080` | URL agents/installers use to reach the API                                                                        |
| `FLEETDOCK_AGENT_BIN_DIR`                             | `/opt/fleetdock/agents` | where cross-compiled agent binaries live                                                                          |
| `FLEETDOCK_WORKER_ENABLED`                            | `true`                  | in-process worker (external-instance ops, offline detection, scheduled backups, retention, alerts, notifications) |
| `FLEETDOCK_HEARTBEAT_TIMEOUT`                         | `2m`                    | no heartbeat for this long ⇒ server `offline`                                                                     |
| `FLEETDOCK_METRICS_RETENTION`                         | `168h`                  | how long per-heartbeat server metrics history is kept                                                             |
| `FLEETDOCK_SMTP_HOST`                                 | —                       | SMTP host for email notification channels (empty ⇒ email delivery disabled)                                       |
| `FLEETDOCK_SMTP_PORT`                                 | `587`                   | SMTP port                                                                                                         |
| `FLEETDOCK_SMTP_USERNAME` / `FLEETDOCK_SMTP_PASSWORD` | —                       | SMTP auth (optional)                                                                                              |
| `FLEETDOCK_SMTP_FROM`                                 | `fleetdock@localhost`   | envelope/from address for emails                                                                                  |
| `FLEETDOCK_ADMIN_EMAIL` / `FLEETDOCK_ADMIN_PASSWORD`  | —                       | first-run bootstrap only; generate with `./scripts/generate-secrets.sh`                                           |
| `FLEETDOCK_CORS_ORIGIN`                               | `http://localhost:3000` | only used for split-origin deployments; same-origin installs never hit CORS                                       |
| `FLEETDOCK_UI_DIR`                                    | —                       | the bundled dashboard. Set by the image; empty means API-only (bare binary, dev)                                  |
| `FLEETDOCK_UI_PORT`                                   | `3000`                  | loopback port the dashboard binds; never published                                                                |
| `FLEETDOCK_ALLOW_LOOPBACK_DB_HOSTS`                   | `true` in development, `false` in production | let instances point at loopback addresses (link-local, metadata and the metadata DB are always refused) |
| `FLEETDOCK_POSTGRES_PASSWORD`                         | `fleetdock`             | compose only: password of the bundled metadata Postgres; `install.sh` generates one for new installs — never change it on an existing volume |
