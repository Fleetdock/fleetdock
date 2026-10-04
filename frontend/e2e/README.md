# End-to-end tests

Playwright tests that drive a **running** Fleetdock through a browser. They
create servers, users and tokens, so point them at a throwaway install.

```sh
npx playwright install chromium   # once
FLEETDOCK_E2E_URL=http://127.0.0.1:18080 \
FLEETDOCK_E2E_EMAIL=admin@example.com \
FLEETDOCK_E2E_PASSWORD=… \
npm run e2e
```

The discovery test also needs a PostgreSQL that the Fleetdock container can
reach, plus the name of its docker container so the test can create and drop
databases behind Fleetdock's back:

```sh
docker run -d --name fd-e2e-pg --network <fleetdock network> -e POSTGRES_PASSWORD=pw postgres:16-alpine
FLEETDOCK_E2E_PG_HOST=fd-e2e-pg FLEETDOCK_E2E_PG_PASSWORD=pw FLEETDOCK_E2E_PG_CONTAINER=fd-e2e-pg npm run e2e
```

It waits for the "not found on server" state, which takes about four minutes.
