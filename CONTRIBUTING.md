# Contributing to db-manager

Thank you for your interest in contributing! This document covers how to set up a development environment, run tests, and submit changes.

## Getting Started

1. Fork the repository on GitHub.
2. Clone your fork and create a feature branch:
   ```bash
   git clone git@github.com:YOUR_USER/db-manager.git
   cd db-manager
   git checkout -b my-feature
   ```
3. Copy the environment template and generate secrets:
   ```bash
   cp .env.example .env
   ./scripts/generate-secrets.sh >> .env
   ```
4. Start the full stack (Postgres + API + frontend):
   ```bash
   make up
   ```
   Or run with hot reload for local development:
   ```bash
   make dev
   ```

The dashboard is at http://localhost:3000 and the API at http://localhost:8080.

## Development Requirements

- Go 1.22+
- Node.js 22+
- Docker and Docker Compose (for the full stack)
- `golangci-lint` (for linting; CI installs it automatically)

## Project Layout

```
backend/     Go API, worker, and agent (clean architecture)
frontend/    Next.js dashboard
docs/        OpenAPI specification
scripts/     Helper scripts (secret generation, etc.)
```

Configuration uses the `MDCP_*` environment prefix (MariaDB Control Plane — historical name retained for compatibility).

## Code Style

### Go

- Run `make fmt` before committing.
- Follow existing patterns: domain types in `internal/domain/`, services in `internal/app/`, HTTP handlers in `internal/interfaces/httpapi/`.
- Use the fake-repo pattern for unit tests (see `backend/internal/app/server/service_test.go`).
- Prefer explicit error handling; use `apperr` for domain errors.

### TypeScript / Frontend

- Run `npm run lint` and `npm run typecheck` in `frontend/`.
- Use existing UI components from `frontend/src/components/`.
- Keep API calls in `frontend/src/lib/api.ts`.

## Testing

```bash
make test          # go vet + backend unit tests
make lint          # golangci-lint + ESLint
make build         # compile backend and frontend
```

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

If you add or modify HTTP routes, update [docs/openapi.yaml](docs/openapi.yaml) and ensure the OpenAPI drift test passes.

## Security

See [SECURITY.md](SECURITY.md) for vulnerability reporting. Do not commit secrets, `.env` files, or credentials.

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). Be respectful and constructive in all interactions.
