# MariaDB Control Plane — API service

Go control-plane API. Increment 1 implements the **Servers** resource
(register / list / get) as a full vertical slice through clean architecture.

## Layout

```
services/api/
  cmd/api/                 entrypoint + dependency injection
  internal/
    config/                env-based configuration
    platform/apperr/       transport-agnostic error model
    domain/server/         entity + Repository port (no deps on infra/http)
    app/server/            use cases (Register/Get/List) + unit tests
    infra/postgres/        pgx pool, embedded migration runner, repository
    interfaces/httpapi/    router, handlers, DTOs, middleware + tests
```

Dependencies flow inward only: `interfaces` and `infra` depend on `app` and
`domain`; `domain` depends on nothing but the shared error type.

## Run locally

```bash
docker compose up --build
# API on http://localhost:8080, migrations applied on boot
```

Or run the binary against your own Postgres:

```bash
cp .env.example .env            # edit MDCP_DATABASE_URL
export $(grep -v '^#' .env | xargs)
go run ./cmd/api
```

## API

| Method | Path              | Description                              |
|--------|-------------------|------------------------------------------|
| GET    | `/healthz`        | Liveness probe                           |
| POST   | `/v1/servers`     | Register a server                        |
| GET    | `/v1/servers`     | List servers (`status`,`search`,`tag`,`limit`,`offset`) |
| GET    | `/v1/servers/{id}`| Get one server                           |

Register:

```bash
curl -sX POST localhost:8080/v1/servers \
  -H 'content-type: application/json' \
  -d '{"name":"db-1","hostname":"db1.internal","tags":["prod"],"labels":{"region":"eu"}}'
```

Error envelope (consistent across endpoints):

```json
{ "error": { "code": "invalid", "message": "name is required", "field": "name" } }
```

Status codes: `422` invalid, `404` not found, `409` conflict, `500` internal.

## Test

```bash
go test ./...
```

Unit tests cover the application layer (fake repository) and the HTTP layer
(fake service). The Postgres repository is exercised by integration tests
against a real database in CI.
