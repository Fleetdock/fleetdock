# Operations runbook

Day-2 procedures for running Fleetdock in production: backing up metadata,
upgrading, rotating encryption keys, and monitoring health.

See also [DEPLOYMENT.md](DEPLOYMENT.md) and [SECURITY-CHECKLIST.md](SECURITY-CHECKLIST.md).

## What to protect

The **metadata PostgreSQL database** is the source of truth for:

- User accounts, roles, and API tokens
- Server and agent enrollment state
- Instance connection credentials (encrypted)
- Backup destinations (encrypted S3 keys)
- Operation/job history

Losing this database without backups means re-enrolling every server and
re-entering every external instance credential.

Backup **objects** in S3/R2 are separate; Fleetdock stores pointers and
checksums in metadata.

## Back up the metadata database

### Managed Postgres (recommended)

Use your provider's automated backup (Neon PITR, RDS snapshots, etc.). Verify
restore in a staging environment at least once.

### Installed with install.sh

```bash
fleetdock backup-db                        # → /opt/fleetdock/backups/metadata-<time>.sql.gz
fleetdock backup-db /mnt/offsite/meta.sql.gz
```

`fleetdock update` takes the same backup automatically before upgrading and
stops if it fails (override with `FLEETDOCK_SKIP_BACKUP=1`). Restore into an
empty database with `gunzip -c metadata-….sql.gz | fleetdock psql`.

Take one every night, keep two weeks, and copy them off the host (a backup on
the same disk does not survive the disk). For example, in root's crontab:

```cron
17 3 * * * /usr/local/bin/fleetdock backup-db --keep 14 && aws s3 sync /opt/fleetdock/backups s3://my-bucket/fleetdock-metadata/
```

(`rclone sync` or `restic` work the same way.)

Stored credentials in the dump are encrypted with `FLEETDOCK_ENCRYPTION_KEY`:
keep `fleetdock backup-config` (the `.env`) alongside, or they are unreadable.

### Self-hosted Postgres (Compose `postgres` service)

Nightly logical dump:

```bash
docker compose exec -T postgres \
  pg_dump -U fleetdock -d fleetdock --no-owner --format=custom \
  > "fleetdock-$(date +%F).dump"
```

Copy dumps off-host (S3, another server). Retain 30 days minimum.

Restore (empty database):

```bash
docker compose exec -T postgres \
  pg_restore -U fleetdock -d fleetdock --clean --if-exists \
  < fleetdock-2026-07-10.dump
```

Stop the API during restore to avoid concurrent writes.

## Upgrading Fleetdock

### Installed with install.sh

```bash
fleetdock update              # latest
fleetdock update --tag v1.0.0 # a specific release
```

### Compose (build from source)

```bash
git fetch origin
git checkout v1.0.0   # or main for latest
docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d
```

Migrations run automatically on API start (`FLEETDOCK_RUN_MIGRATIONS=true`, the
default). To apply migrations without serving traffic:

```bash
docker compose run --rm fleetdock /api   # or: make backend-migrate locally
```

Set `FLEETDOCK_RUN_MIGRATIONS=false` only if you run migrations as a separate
job. A process started that way checks the schema and refuses to start while
migrations are pending, so it never serves against a schema it does not match.

The API also refuses to start in two cases, with a message saying what to do:

- **The metadata database was migrated by a newer Fleetdock.** It would run
  against tables it does not know. Run the newer version, or
  roll back properly (below).
- **A migration that was already applied has changed.** Fleetdock records each
  migration's SHA-256; a different file means the schema may not match the
  code.

### Which version is running

The dashboard shows it at the bottom of the sidebar. From the shell:

```bash
fleetdock version                           # install.sh installs
docker compose exec fleetdock /api version  # compose
```

The API reports it at `GET /v1/version` (signed in), with the oldest agent
version it supports.

### Upgrade checklist

1. Read [CHANGELOG.md](../CHANGELOG.md) for breaking changes
2. Back up metadata Postgres (above); `fleetdock update` does this for you
3. Deploy new images / rebuild
4. Confirm `GET /readyz` returns `ready` and the sidebar shows the new version
5. Spot-check: sign in, open the server list and a database
6. Watch **Activity** for stuck tasks after the upgrade
7. Update the agents (below)

### Updating agents

Agents keep working across a control-plane upgrade, but each release may fix
them too. A server whose agent is older than the control plane shows
**Update** in the server list, an *Agent update available* notice on its page
and an item on the overview; an agent older than the oldest supported version
is marked *too old*.

To update one, run this on the machine:

```bash
curl -sSL https://fleetdock.example.com/install.sh | sudo sh
```

Without a registration token, the installer updates the enrolled agent in
place: it downloads the binary from your Fleetdock, checks it against the
SHA-256 the control plane publishes (`/agent/v1/binary/{os}/{arch}/sha256`),
replaces it and restarts the service. The server keeps its identity and
token. The new version shows on the server's page after the next heartbeat.

### Rollback

1. Stop the new API container
2. Restore the metadata database backup taken before upgrade
3. Redeploy the previous image tag

Do not roll back only the API binary if migrations already advanced: from 1.0
on, an older version refuses to start on the newer schema (earlier versions
start and misbehave). Restore the database to match the older version, or stay
on the new one.

## Encryption key rotation

Stored credentials (database server passwords, SSH keys, backup storage keys,
notification webhook URLs and SMTP passwords) are **envelope-encrypted**: each
has its own data key, wrapped by `FLEETDOCK_ENCRYPTION_KEY`. Rotating the key
re-wraps the data keys only; the encrypted payloads are untouched.

### Installed with install.sh

```bash
fleetdock rotate-keys
```

It backs up the metadata database and `.env`, generates a new key under a new
id, restarts with the old key still readable, re-wraps every secret, then drops
the old key. If anything fails, the previous `.env` is put back. Afterwards
keep a copy of the new `.env` (`fleetdock backup-config`): metadata backups
taken before the rotation still need the old key, which is saved next to them
as `backups/env-before-rotate-<time>`.

### Compose, or the API binary

**Never reuse a key id with a different secret.**

1. Keep the current key readable and promote a new one under a new id:

   ```bash
   FLEETDOCK_ENCRYPTION_KEYS_OLD="master-1=<current key>"   # comma-separate several
   FLEETDOCK_ENCRYPTION_KEY="<new key, 32+ random bytes, base64>"
   FLEETDOCK_ENCRYPTION_KEY_ID="master-2"
   ```

   (`master-1` is the default id when `FLEETDOCK_ENCRYPTION_KEY_ID` was never set.)

2. Restart the control plane with these settings, then re-wrap:

   ```bash
   docker compose exec fleetdock /api rotate-keys   # or: make rotate-keys
   ```

   It reports how many secrets it re-wrapped; run again, it reports them all
   current.

3. Remove `FLEETDOCK_ENCRYPTION_KEYS_OLD` and restart.

## JWT secret rotation

Rotating `FLEETDOCK_JWT_SECRET` signs everyone out of the dashboard. API tokens
(`fleetd_…`) and agent tokens are not affected: they are random secrets looked
up by hash, not signed with it. To revoke API tokens, revoke them under
**Access → API tokens**.

1. Announce the sign-out to users
2. Update `FLEETDOCK_JWT_SECRET` and restart the control plane
3. Users sign in again

## Monitoring

### Health endpoints and metrics

| Endpoint | Auth | Use |
|----------|------|-----|
| `GET /healthz` | None | Liveness — process is up |
| `GET /readyz` | None | Readiness — the metadata database answers and, where the worker runs, it has turned in the last 2 minutes |
| `GET /metrics` | `FLEETDOCK_METRICS_TOKEN` as a bearer token | Prometheus metrics; 404 while the token is unset |

Point your load balancer or uptime checker at `/healthz`. Use `/readyz` for
dependency-aware routing.

`/metrics` reports the build (`fleetdock_build_info`), HTTP requests and
latency, operations finished by type and result
(`fleetdock_operations_finished_total`), when the worker last ran
(`fleetdock_worker_last_run_seconds`), and fleet gauges: servers online and
offline, database servers, databases, backups and failed backups in the last
24 hours, operations running, the time of the last backup. Set a long random
`FLEETDOCK_METRICS_TOKEN` (`fleetdock config set FLEETDOCK_METRICS_TOKEN …`)
and scrape with:

```yaml
scrape_configs:
  - job_name: fleetdock
    scheme: https
    authorization: {credentials: "<FLEETDOCK_METRICS_TOKEN>"}
    static_configs: [{targets: ["fleetdock.example.com"]}]
```

### What to alert on

- `/readyz` non-200 for > 2 minutes
- Disk full on the host (agent heartbeats and metrics accumulate)
- Backup operations failing (dashboard **Operations** or notification rules)
- Servers stuck `offline` (heartbeat timeout default: 2 minutes)
- Instances whose health is `unreachable` (the dashboard shows the probe result
  per instance; managed instances the control plane cannot reach directly show
  `unknown`, which is not an outage)
- Backup verifications that fail (`backup.verify_failed` notification event)

Structured JSON logs go to stdout from the API — ship them to your log stack.

### Worker

The in-process worker (`FLEETDOCK_WORKER_ENABLED=true`) handles external-instance
operations, scheduled backups, retention pruning, offline detection, the
instance health probe and notifications.

Several API replicas may share one metadata database: jobs are claimed with
`FOR UPDATE SKIP LOCKED`, the periodic housekeeping runs on one replica at a
time (PostgreSQL advisory lock) and migrations serialise. Two caveats: cancelling
a running SQL console query only works when the request reaches the replica that
runs it (use sticky sessions), and operations orphaned by a crashed replica are
failed automatically after 2h15m (the maximum run time plus margin).

## Admin password reset

Prefer the dashboard **Users** page when any admin account still works.
If you are locked out:

```bash
fleetdock reset-admin-password                    # the bootstrap admin email
fleetdock reset-admin-password ops@example.com    # any dashboard user
```

It prints a new generated password, reactivates the account if it was
suspended and signs out all of its sessions. Without the CLI (Compose from
source, Kubernetes), run the same thing in the control-plane container:

```bash
docker compose exec fleetdock /api reset-password admin@example.com
# or choose the password:
docker compose exec -e FLEETDOCK_NEW_PASSWORD='…' fleetdock /api reset-password admin@example.com
```

It needs only `FLEETDOCK_DATABASE_URL`, so it works even when the API does not
start.

## Data retention

| Data | Default retention | Config |
|------|-------------------|--------|
| Server metrics samples | 7 days | `FLEETDOCK_METRICS_RETENTION` |
| Audit log | 365 days | `FLEETDOCK_AUDIT_RETENTION` (`0` keeps everything) |
| Row changes (data browser undo) | 7 days | `FLEETDOCK_ROW_CHANGE_RETENTION` |
| Fleet query runs and results | 30 days | `FLEETDOCK_FLEET_RUN_RETENTION` |
| Opened backup copies | 2 hours | `FLEETDOCK_BACKUP_COPY_TTL` |
| Soft-deleted databases | 7 days | Built-in recovery window |
| Backup objects | Per schedule / manual | Destination + schedule retention |
| Metadata backups (`fleetdock backup-db`) | Until removed | `--keep N` |

## Support and releases

- Security issues: [SECURITY.md](../SECURITY.md)
- Version support: latest `0.1.x` on `main`
- Release artifacts: GitHub Releases + GHCR images (see [DEPLOYMENT.md](DEPLOYMENT.md))
