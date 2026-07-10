# Migrating from db-manager to Fleetdock

Fleetdock is the new product name for the former **db-manager** project.

## Environment variables

| New (preferred) | Legacy alias (still works) |
|-----------------|----------------------------|
| `FLEETDOCK_DATABASE_URL` | `MDCP_DATABASE_URL` |
| `FLEETDOCK_JWT_SECRET` | `MDCP_JWT_SECRET` |
| `FLEETDOCK_ENCRYPTION_KEY` | `MDCP_ENCRYPTION_KEY` |
| `FLEETDOCK_PUBLIC_URL` | `MDCP_PUBLIC_URL` |
| `FLEETDOCK_URL` (agent) | `MDCP_URL` |
| `FLEETDOCK_TOKEN` (agent) | `MDCP_TOKEN` |
| … | all other `MDCP_*` vars |

Update `.env` at your convenience; aliases are read when the `FLEETDOCK_*` name is unset.

## Agent install

New install command:

```bash
curl -sSL https://fleetdock.dev/install.sh | \
  FLEETDOCK_URL=https://fleetdock.dev FLEETDOCK_TOKEN=fleetr_... sh
```

Legacy `MDCP_URL` / `MDCP_TOKEN` still work in the install script.

Installed paths changed:

| Before | After |
|--------|-------|
| `/usr/local/bin/db-manager-agent` | `/usr/local/bin/fleetdock-agent` |
| `/etc/db-manager-agent/` | `/etc/fleetdock-agent/` |
| `db-manager-agent.service` | `fleetdock-agent.service` |

Re-run the install script on managed servers or migrate the systemd unit manually.

## API tokens

New user API tokens use the `fleetd_` prefix. Existing `mdcp_` tokens remain valid until revoked.

Registration tokens: `fleetr_` (was `mdcpr_`). Agent tokens: `fleeta_` (was `mdcpa_`).

## Container images

```text
ghcr.io/tajbrains/fleetdock-backend:<tag>
ghcr.io/tajbrains/fleetdock-frontend:<tag>
```

## Go module

`github.com/TajBrains/fleetdock/backend` (was `github.com/TajBrains/db-manager/backend`).

## GitHub repository

Rename the repository to `fleetdock` in GitHub settings when ready. Until then, clone URL may still show `db-manager`.
