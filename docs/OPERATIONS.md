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
- Operation/job history and audit log

Losing this database without backups means re-enrolling every server and
re-entering every external instance credential.

Backup **objects** in S3/R2 are separate; Fleetdock stores pointers and
checksums in metadata.

## Back up the metadata database

### Managed Postgres (recommended)

Use your provider's automated backup (Neon PITR, RDS snapshots, etc.). Verify
restore in a staging environment at least once.

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

### Compose (build from source)

```bash
git fetch origin
git checkout v0.2.0   # or main for latest
docker compose pull   # if using GHCR images
docker compose -f docker-compose.yml -f docker-compose.prod.yml up --build -d
```

Migrations run automatically on API start (`FLEETDOCK_RUN_MIGRATIONS=true`, the
default). To apply migrations without serving traffic:

```bash
docker compose run --rm backend /api   # or: make backend-migrate locally
```

Set `FLEETDOCK_RUN_MIGRATIONS=false` only if you run migrations as a separate job.

### Upgrade checklist

1. Read [CHANGELOG.md](../CHANGELOG.md) for breaking changes
2. Back up metadata Postgres (above)
3. Deploy new images / rebuild
4. Confirm `GET /readyz` returns `ready`
5. Spot-check: login, server list, trigger a connection test on an instance
6. Watch the **Operations** page for stuck jobs after upgrade

### Rollback

1. Stop the new API container
2. Restore the metadata database backup taken before upgrade
3. Redeploy the previous image tag

Do not roll back only the API binary if migrations already advanced — restore
the database to match the older schema or stay on the new version.

## Encryption key rotation

Instance passwords and S3 destination keys are **envelope-encrypted**. Rotating
`FLEETDOCK_ENCRYPTION_KEY` re-wraps data keys only; ciphertext payloads are untouched.

**Never reuse a key id with a different secret.**

### Steps

1. Generate a new secret (32+ random bytes, base64 or hex)
2. Set environment before running the rotation job:

```bash
export FLEETDOCK_ENCRYPTION_KEYS_OLD="master-1=<current-secret>"
export FLEETDOCK_ENCRYPTION_KEY="<new-secret>"
export FLEETDOCK_ENCRYPTION_KEY_ID="master-2"
```

3. Run the offline re-wrap (API can stay up; rotation is read/write on secrets table):

```bash
make rotate-keys
# or inside the backend container:
docker compose exec backend /api  # not applicable — use migrate binary
docker compose run --rm -e FLEETDOCK_DATABASE_URL -e FLEETDOCK_ENCRYPTION_KEY \
  -e FLEETDOCK_ENCRYPTION_KEY_ID -e FLEETDOCK_ENCRYPTION_KEYS_OLD \
  backend sh -c 'go run ./cmd/rotate-keys'  # if Go toolchain in image
```

The production image contains only the compiled API. Run rotation from a checkout
or a one-off container with the repo:

```bash
cd backend && go run ./cmd/rotate-keys
```

4. When the tool reports **0 secrets remaining** under the old key, update
   production env: remove `FLEETDOCK_ENCRYPTION_KEYS_OLD`, keep `master-2` as primary
5. Restart the API

Full details: [README — Security](../README.md#security).

## JWT secret rotation

Rotating `FLEETDOCK_JWT_SECRET` **invalidates all active sessions and API tokens**
derived from user login (not agent tokens). Plan a maintenance window:

1. Announce logout to users
2. Update `FLEETDOCK_JWT_SECRET` and restart API
3. Users log in again; revoke and recreate long-lived API tokens

## Monitoring

### Health endpoints

| Endpoint | Auth | Use |
|----------|------|-----|
| `GET /healthz` | None | Liveness — process is up |
| `GET /readyz` | None | Readiness — metadata DB ping succeeds |

Point your load balancer or uptime checker at `/healthz`. Use `/readyz` for
dependency-aware routing.

### What to alert on

- `/readyz` non-200 for > 2 minutes
- Disk full on the host (agent heartbeats and metrics accumulate)
- Backup operations failing (dashboard **Operations** or notification rules)
- Servers stuck `offline` (heartbeat timeout default: 2 minutes)

Structured JSON logs go to stdout from the API — ship them to your log stack.

### Worker

The in-process worker (`FLEETDOCK_WORKER_ENABLED=true`) handles external-instance
operations, scheduled backups, retention pruning, offline detection, and
notifications. Only one API instance should run the worker per metadata database
unless you implement external job locking (not supported in v0.1.x).

## Admin password reset

If locked out and metadata DB is intact:

1. Connect to Postgres
2. Set a new bcrypt hash on the user row, **or**
3. Delete all rows from `users` and restart API with `FLEETDOCK_ADMIN_*` set to
   bootstrap a fresh admin (destructive — only for greenfield recovery)

Prefer the dashboard **Users** page when any admin account still works.

## Data retention

| Data | Default retention | Config |
|------|-------------------|--------|
| Server metrics samples | 7 days | `FLEETDOCK_METRICS_RETENTION` |
| Soft-deleted databases | 7 days | Built-in recovery window |
| Backup objects | Per schedule / manual | Destination + schedule retention |
| Audit log | Indefinite | Export manually if needed |

## Support and releases

- Security issues: [SECURITY.md](../SECURITY.md)
- Version support: latest `0.1.x` on `main`
- Release artifacts: GitHub Releases + GHCR images (see [DEPLOYMENT.md](DEPLOYMENT.md))
