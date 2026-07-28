# AGENTS.md

## Cursor Cloud specific instructions

Product: **Fleetdock**, a self-hosted database control plane. Three parts run
locally for development: the Go API (`backend/cmd/api`, port **8080**, includes
the in-process operations worker), the Next.js dashboard (`frontend`, port
**3000**), and a **PostgreSQL 16** metadata database (port **5432**). Standard
lint/test/build/run commands live in the root `Makefile` and
`frontend/package.json`; see also `README.md`. Below are only the non-obvious
caveats for this environment.

### Toolchain locations
- Go **1.25** is installed at `/usr/local/go/bin` and is prepended to `PATH`
  via `~/.bashrc`. The distro also ships `/usr/bin/go` (**1.22**), which is too
  old for `go.mod` (`go 1.25`) — make sure `/usr/local/go/bin` wins on `PATH`
  (non-login shells that skip `~/.bashrc` may need `export PATH=/usr/local/go/bin:$HOME/go/bin:$PATH`).
- `golangci-lint` **v2.12.2** (matches CI) is at `~/go/bin/golangci-lint`.

### PostgreSQL (not Docker)
- Postgres runs as a local system cluster, not via `docker compose`. It is
  **not auto-started** on boot — start it with `sudo pg_ctlcluster 16 main start`
  (check with `pg_lsclusters`).
- Credentials/database (created during setup): user `fleetdock`, password
  `fleetdock`, database `fleetdock`, on `localhost:5432`.

### Environment files (git-ignored, must exist)
- Root `.env` is required by `make dev` / `make backend-run`. Create it with
  `cp .env.example .env && ./scripts/generate-secrets.sh >> .env`, then change
  `FLEETDOCK_DATABASE_URL` host from `@postgres:5432` (Docker service name) to
  `@localhost:5432` for local, non-Docker dev.
- `frontend/.env.local` is optional — the dashboard calls the API on its own
  origin, and `next dev` proxies the API paths to `:8080` for you.

### Running
- `make dev` loads root `.env` and runs API + Next.js with hot reload.
- Dashboard: http://localhost:3000 — log in as `admin@example.com` with the
  `FLEETDOCK_ADMIN_PASSWORD` value from `.env` (the API bootstraps this admin on
  first boot when the users table is empty).
- Browser requests go to `/v1/...` on port 3000 and are rewritten to the Go API
  (see `rewrites()` in `frontend/next.config.mjs`). In production the same paths
  are served by the Go binary directly — same origin either way. Never
  re-introduce a build-time API host.
- `/healthz` is an unauthenticated liveness check; `/readyz` is an unauthenticated
  readiness check (metadata DB ping).

### Not installed by default
- **Docker** is not installed, so `docker compose up` (full-stack path) and the
  server-agent provisioning flow (managed instances, backups to S3/R2, database
  moves) cannot run without installing Docker + target databases first. The
  core control plane (login, dashboard, users/roles, external-instance
  metadata) works without them. Backend unit tests (`make test`) do not need
  Postgres.
