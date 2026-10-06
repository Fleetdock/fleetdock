# Contributing to Fleetdock

Thank you for your interest in contributing! This document covers how to set up a development environment, run tests, and submit changes.

## Getting Started

1. Fork the repository on GitHub.
2. Clone your fork and create a feature branch:
   ```bash
   git clone git@github.com:YOUR_USER/Fleetdock.git
   cd Fleetdock
   git checkout -b my-feature
   ```
3. Copy the environment template and generate secrets:
   ```bash
   cp .env.example .env
   ./scripts/generate-secrets.sh >> .env
   ```
4. Run it, one of two ways:

   | Command | What runs | Open |
   | --- | --- | --- |
   | `make dev` | The Go API and the Next.js dev server on your machine, with hot reload. Needs Go, Node and a PostgreSQL ([below](#without-docker)). | http://localhost:3000 |
   | `make up` | The real stack — Caddy, Fleetdock built from this checkout, PostgreSQL — in Docker. | http://localhost |

   Sign in as `FLEETDOCK_ADMIN_EMAIL` with the `FLEETDOCK_ADMIN_PASSWORD` from
   `.env`. The API applies migrations and creates that account on first boot.

`make help` lists every target. To try the installer itself, run it in a
throwaway VM — see [DEPLOYMENT.md → Trying it in a VM](docs/DEPLOYMENT.md#trying-it-in-a-vm).

### Without Docker

The API needs a PostgreSQL 14+ it can reach. Under Docker the stack supplies
one; for `make dev`, add its address to `.env`:

```bash
FLEETDOCK_DATABASE_URL=postgres://fleetdock:fleetdock@localhost:5432/fleetdock?sslmode=disable
```

`make dev` then loads `.env` and runs both servers. The Next.js dev server
proxies `/v1`, `/agent` and the other API paths to the Go API on `:8080`, so the
dashboard is same-origin in development exactly as in production. Point it
elsewhere with `FLEETDOCK_DEV_API_URL`.

To run an agent by hand against a dev control plane:

```bash
cd backend
go build -o fleetdock-agent ./cmd/agent
FLEETDOCK_URL=http://localhost:8080 FLEETDOCK_TOKEN=<registration-token> \
  FLEETDOCK_STATE_DIR=/tmp/fleetdock-agent ./fleetdock-agent
```

Agents on another machine (a VM, say) need your LAN address instead of
`localhost`, here and in `FLEETDOCK_PUBLIC_URL`.

## Development Requirements

- Go 1.26+
- Node.js 22+
- Docker and Docker Compose (for the full stack)
- `golangci-lint` (for linting; CI installs it automatically)

## Project Layout

```
backend/     Go API, worker, and agent (clean architecture)
frontend/    Next.js dashboard
docs/        Install, operations, security and configuration guides
scripts/     install.sh (the Fleetdock installer), the fleetdock CLI, generate-secrets.sh
```

Configuration uses the `FLEETDOCK_*` environment prefix — see
[docs/CONFIGURATION.md](docs/CONFIGURATION.md).

## Code Style

### Go

- Run `make fmt` before committing.
- Follow existing patterns: domain types in `internal/domain/`, services in `internal/app/`, HTTP handlers in `internal/interfaces/httpapi/`.
- Use the fake-repo pattern for unit tests (see `backend/internal/app/server/service_test.go`).
- Prefer explicit error handling; use `apperr` for domain errors.

### TypeScript / Frontend

- Run `npm run lint` and `npm run typecheck` in `frontend/`.
- Use existing UI components from `frontend/src/components/` (`components/ui`
  has the primitives: `PageHeader`, `Modal`, `Field`, `Menu`, `Time`, …).
- Data hooks live in `frontend/src/lib/data/<area>.ts` (re-exported from
  `@/lib/hooks`); the HTTP client is `frontend/src/lib/api.ts`.
- Write user-facing text with the words in [docs/glossary.md](docs/glossary.md).
- Pages and sections come from `frontend/src/lib/nav.ts`; moving a page needs
  an entry in `frontend/src/lib/legacy-redirects.json`.

## Testing

```bash
make test          # go vet + backend unit tests
make lint          # golangci-lint + ESLint
make build         # compile backend and frontend
```

Frontend unit tests: `npm test` in `frontend/`. Browser tests drive a running
install — see [frontend/e2e/README.md](frontend/e2e/README.md). Repository
tests against a real PostgreSQL:
`FLEETDOCK_IT_POSTGRES=127.0.0.1:5432 go test -tags integration ./internal/infra/postgres/`.

Add tests for new service logic. Focus on validation, state transitions, and error paths.

## Pull Requests

1. Ensure `make lint && make test && make build` pass.
2. Update [CHANGELOG.md](CHANGELOG.md) under `Unreleased` if the change is user-facing.
3. Open a PR against `main` using the PR template.
4. Describe what changed and how you tested it.

### Commit Messages

Use clear, imperative subject lines:

- `Add backup retention pruning to worker`
- `Fix instance provisioning port validation`

Reference issue numbers when applicable (`Fixes #123`).

## API Changes

If you add or modify HTTP routes, update
[`backend/internal/openapi/openapi.yaml`](backend/internal/openapi/openapi.yaml)
and ensure the OpenAPI drift test passes.

## Security

See [SECURITY.md](SECURITY.md) for vulnerability reporting. Do not commit secrets, `.env` files, or credentials.

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). Be respectful and constructive in all interactions.
