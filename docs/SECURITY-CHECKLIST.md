# Production security checklist

Use this list before exposing Fleetdock to the internet or untrusted networks.
See [SECURITY.md](../SECURITY.md) for vulnerability reporting.

## Network exposure

```text
                    Internet / corp network
                              │
                    ┌─────────▼─────────┐
                    │  TLS reverse proxy │
                    │  (443 only public) │
                    └─────────┬─────────┘
                              │
              ┌───────────────┼───────────────┐
              │               │               │
        Dashboard         API            Metadata DB
        (frontend)      (backend)        (Postgres)
                              │
              ┌───────────────┴───────────────┐
              │                               │
        Managed servers                  External DBs
        (agent → API)              (API → host:port)
```

### Control plane (dashboard + API)

- [ ] **HTTPS only** — terminate TLS at the reverse proxy; do not expose :8080/:3000 publicly
- [ ] **Firewall** — allow 443 to the proxy; deny direct access to Postgres from the internet
- [ ] **Production mode** — `FLEETDOCK_ENV=production` (refuses default secrets)
- [ ] **Strong secrets** — run `./scripts/generate-secrets.sh`; never commit `.env`
- [ ] **Bootstrap admin** — change `FLEETDOCK_ADMIN_PASSWORD` after first login; use a dedicated admin email
- [ ] **CORS** — `FLEETDOCK_CORS_ORIGIN` matches exactly one dashboard origin (no `*`)

### Managed servers (agent)

- [ ] **`FLEETDOCK_PUBLIC_URL`** is the URL agents use — must be reachable from every managed host on 443
- [ ] Registration tokens are **single-use** and short-lived; do not share install commands in public channels
- [ ] Agent bearer tokens are stored on disk (`state.json` on the server) — protect file permissions (systemd runs as root today; restrict backup access to state dir)

### External database instances

- [ ] The control plane connects **directly** to external DB host:port for backups, restores, and live admin
- [ ] Ensure the API host can reach those ports (security group / firewall rule)
- [ ] Prefer private network paths (VPC peering, WireGuard) over public internet
- [ ] Use least-privilege DB users for instance admin credentials stored in Fleetdock
- [ ] **Cloudflare Tunnel / TCP proxy** for closed-port access is on the roadmap — not in v0.1.x

## Authentication and sessions

- [ ] **JWT in localStorage** — the dashboard stores the session token in `localStorage`, which is vulnerable to XSS. Mitigations:
  - Keep frontend dependencies updated (`npm audit`, Dependabot)
  - Do not install untrusted browser extensions for admin users
  - Consider placing the dashboard behind VPN, SSO gateway, or IP allowlist until httpOnly cookies or OIDC land
- [ ] **API tokens** — scope tokens minimally; set expiry; revoke unused tokens
- [ ] **RBAC** — use `viewer` / `operator` roles instead of giving everyone `owner`
- [ ] **Suspend** compromised accounts immediately (sessions stop working)
- [ ] **SSO/OIDC and 2FA** — not yet available; track [ROADMAP.md](../ROADMAP.md)

## Secrets at rest

- [ ] **`FLEETDOCK_ENCRYPTION_KEY`** — unique per environment; back up rotation procedure ([OPERATIONS.md](OPERATIONS.md))
- [ ] **`FLEETDOCK_JWT_SECRET`** — unique per environment; rotation logs everyone out
- [ ] Metadata Postgres — encrypted storage at rest (provider feature or disk encryption)
- [ ] S3/R2 destination keys — encrypted in DB; agents receive presigned URLs only for backup/restore

## Metadata database

- [ ] Dedicated database user with minimum required privileges
- [ ] `sslmode=require` (or stricter) for remote Postgres
- [ ] Automated backups and tested restore ([OPERATIONS.md](OPERATIONS.md))
- [ ] No public ingress to Postgres port 5432

## Backups and object storage

- [ ] Backup buckets are **private**; presigned URLs are time-limited
- [ ] Separate IAM/bucket per environment (prod vs staging)
- [ ] Enable bucket versioning or lifecycle rules as your compliance requires

## Observability and audit

- [ ] Monitor `/healthz` and `/readyz`
- [ ] Review **Operations** log for unexpected mutating actions
- [ ] Configure notification channels for backup failures and server offline events
- [ ] Ship API JSON logs to a central system; restrict log access (credentials may appear in error messages — avoid logging request bodies)

## Supply chain

- [ ] Pin image tags in production (`v0.1.0`, not `latest`)
- [ ] Verify images from `ghcr.io/tajbrains/Fleetdock-*` or build yourself from tagged source
- [ ] Enable GitHub Dependabot / renovate for the fork

## Incident response

- [ ] Document who can rotate secrets and restore metadata DB
- [ ] Know how to revoke all API tokens (JWT secret rotation or per-token revoke in UI)
- [ ] Report vulnerabilities privately per [SECURITY.md](../SECURITY.md)

## Quick pre-flight command

```bash
# Should fail in production with default secrets:
FLEETDOCK_ENV=production FLEETDOCK_DATABASE_URL=postgres://x:x@localhost/x \
  FLEETDOCK_JWT_SECRET=dev-insecure-change-me \
  FLEETDOCK_ENCRYPTION_KEY=dev-insecure-encryption-key \
  FLEETDOCK_ADMIN_PASSWORD=admin12345 \
  go run ./cmd/api
# Expected: refusing to start: FLEETDOCK_JWT_SECRET uses an insecure default
```

Run from `backend/` after cloning the repo.
