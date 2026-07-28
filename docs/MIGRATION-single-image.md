# Migrating an existing install

Fleetdock now ships as **one image** behind **one domain**, installed with one
command. If you deployed the separate `fleetdock-backend` and
`fleetdock-frontend` images, this is what changes and what you have to do by
hand.

## What changed

| Before                                              | Now                                                                            |
| --------------------------------------------------- | ------------------------------------------------------------------------------ |
| `ghcr.io/fleetdock/fleetdock-backend` + `-frontend` | `ghcr.io/fleetdock/fleetdock` (multi-arch)                                     |
| `docker-compose.yml` + `.prod.yml` + `.ghcr.yml`    | one `docker-compose.yml`, plus `docker-compose.build.yml` to build from source |
| Dashboard on `:3000`, API on `:8080`                | both on `/` behind Caddy on 80/443                                             |
| `NEXT_PUBLIC_API_URL` baked in at build time        | dashboard calls the API on its own origin; nothing baked in                    |
| Your own reverse proxy                              | Caddy bundled, automatic HTTPS                                                 |
| Gateway on by default, 51 published ports           | opt-in via `fleetdock gateway enable`                                          |
| Up to 3 DNS names                                   | 1                                                                              |

## Upgrading

The metadata database schema is unchanged, so the data carries over. From your
existing checkout:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml down   # old stack
```

Then install the new one, pointing it at the **same** Postgres and reusing your
**existing secrets**:

```bash
curl -sSL https://fleetdock.dev/install.sh | sh -s -- --domain db.example.com
```

...then edit `/opt/fleetdock/.env` to carry over from your old `.env`:

- `FLEETDOCK_JWT_SECRET`
- `FLEETDOCK_ENCRYPTION_KEY` — **critical**
- `FLEETDOCK_ENCRYPTION_KEY_ID` and `FLEETDOCK_ENCRYPTION_KEYS_OLD`, if set
- `FLEETDOCK_DATABASE_URL`, if you use hosted Postgres

and restart with `fleetdock restart`.

> `FLEETDOCK_ENCRYPTION_KEY` is not recoverable. Every instance credential,
> application credential and object-store key is envelope-encrypted under it. If
> you install with a fresh key against an existing database, none of them can be
> decrypted and there is no way back. Copy it before you start.

If you were on the bundled Postgres, keep the old volume: point
`FLEETDOCK_DATABASE_URL` at it, or `pg_dump` from the old stack and restore into
the new one.

## Two things that do not migrate automatically

### 1. Enrolled agents pin the old URL

Each managed server has `FLEETDOCK_URL` written into
`/etc/fleetdock-agent/agent.env` at enrolment. The control plane has no job type
that can run an arbitrary command on a managed host, so it **cannot re-point
agents remotely**. Pick one:

- **Keep the old address resolving to this host.** Simplest. Agents keep working
  untouched.
- **Update each server over SSH:**

  ```bash
  sudo sed -i 's|^FLEETDOCK_URL=.*|FLEETDOCK_URL=https://db.example.com|' \
    /etc/fleetdock-agent/agent.env && sudo systemctl restart fleetdock-agent
  ```

- **Re-enrol** with a fresh registration token from **Servers → Connect server**.
  This creates a new server row; historical operations stay on the old one.

`fleetdock domain <new>` prints the exact command for your new hostname.

### 2. Public database endpoints keep their old hostname

`external_host` is stored per endpoint at the moment public access is enabled, so
already-issued connection URLs keep the old name. They keep **working** as long
as that name still resolves to the gateway host — only the display value is
stale. To adopt the new name:

```bash
fleetdock psql -c "UPDATE database_endpoints SET external_host='db.example.com' WHERE external_host='gateway.example.com';"
```

Do **not** disable and re-enable public access instead: that allocates a
different port and breaks every connection string already handed out.

## Other things to check

- Bookmarks and monitors pointing at `:3000` or `:8080` must move to
  `https://<domain>/` and `https://<domain>/healthz`.
- If you deploy on ARM, you can now use the published image — the old ones were
  amd64-only.
- Split-origin deployments (dashboard and API on different hostnames) still work
  but need `NEXT_PUBLIC_API_URL` set as a build argument and your own image
  build. See [DEPLOYMENT.md](DEPLOYMENT.md).
