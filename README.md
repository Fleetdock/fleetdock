# db-manager

An open-source **control plane for MariaDB** — manage a fleet of servers, MariaDB
instances, and databases from a modern dashboard instead of SSH. Think
"Vercel/Neon for MariaDB", not a SQL client.

This is a monorepo:

```
db-manager/
  backend/    Go API (clean architecture, Postgres metadata, JWT + RBAC)
  frontend/   Next.js 16 dashboard (React 19, TypeScript, Tailwind 4, TanStack Query)
  docker-compose.yml
```

## Quickstart

The stack connects to a hosted **Neon** Postgres — there is no local database
container. Configuration lives in a git-ignored root `.env` file (`.env.example`
is the template); set `MDCP_DATABASE_URL` to your Neon connection string, then:

```bash
cp .env.example .env    # then edit MDCP_DATABASE_URL (skip if .env already exists)
docker compose up --build
```

- Dashboard: http://localhost:3000
- API: http://localhost:8080
- Default login: `admin@example.com` / `admin12345` (change via env before production)

The API applies its database migrations and bootstraps the admin account on first
boot. To apply migrations without starting the server: `make backend-migrate`.

## What's implemented

Backend (Go):

- **Auth** — email/password login issuing JWTs, `/auth/me`, bcrypt hashing.
- **RBAC** — permission-based middleware; roles (`owner`, `admin`, `operator`,
  `viewer`) seeded with a permission catalog.
- **API tokens** — create / list / revoke; tokens authenticate like sessions and
  are capped by their scopes.
- **Servers** — register / list / get, labels, tags, search.
- **Instances** — register / list / get under a server.
- **Databases** — create / list / get / lock / unlock / soft-delete (with a
  7-day recovery window).

Frontend (Next.js):

- Login, protected dashboard shell, dark/light theme.
- Servers (list, register, detail with instances), Databases (list, create,
  lock/unlock/delete), API Tokens (create with one-time secret, revoke).

## Architecture

The system is a **control plane** (this repo — decides and records intent) and a
**data plane** (per-server agents that execute against MariaDB). The metadata
schema, orchestration model (Temporal), and agent transport (mTLS gRPC) are
designed for that split; the current code implements the control-plane API and UI
end to end. Long-running operations (real provisioning, backups, restore,
cross-server migration via Temporal workflows and the server agent) are the next
increments.

Backend layering (dependencies point inward):

```
interfaces/httpapi  ->  app/*  ->  domain/*   (domain depends on nothing external)
infra/postgres      ->  app/*  ->  domain/*
```

See `backend/README.md` for the backend details and `backend/internal/infra/postgres/migrations`
for the schema.

## Local development (without Docker)

Backend (needs Go 1.22+ and a Postgres):

```bash
cd backend
export MDCP_DATABASE_URL=postgres://mdcp:mdcp@localhost:5432/mdcp?sslmode=disable
go test ./...
go run ./cmd/api
```

Frontend (needs Node 22+):

```bash
cd frontend
cp .env.local.example .env.local   # NEXT_PUBLIC_API_URL=http://localhost:8080
npm install
npm run dev
```

## Configuration (backend env)

| Variable | Default | Notes |
|----------|---------|-------|
| `MDCP_DATABASE_URL` | — | required |
| `MDCP_HTTP_ADDR` | `:8080` | |
| `MDCP_JWT_SECRET` | dev default | set a strong secret in production |
| `MDCP_ADMIN_EMAIL` / `MDCP_ADMIN_PASSWORD` | `admin@example.com` / `admin12345` | first-run bootstrap only |
| `MDCP_CORS_ORIGIN` | `http://localhost:3000` | frontend origin |
