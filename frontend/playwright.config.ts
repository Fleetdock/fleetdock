import { defineConfig, devices } from "@playwright/test";

/**
 * End-to-end tests against a running Fleetdock (they never start one).
 *
 *   FLEETDOCK_E2E_URL=http://127.0.0.1:18080 \
 *   FLEETDOCK_E2E_EMAIL=admin@example.com FLEETDOCK_E2E_PASSWORD=… \
 *   npm run e2e
 *
 * Point them at a throwaway install: they create servers, users and tokens.
 * See e2e/README.md for the optional discovery test.
 */
export default defineConfig({
  testDir: "e2e",
  testMatch: /.*\.e2e\.ts/,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  reporter: [["list"]],
  use: {
    baseURL: process.env.FLEETDOCK_E2E_URL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    { name: "setup", testMatch: /auth\.setup\.ts/ },
    {
      name: "desktop",
      use: { ...devices["Desktop Chrome"], storageState: "e2e/.auth/admin.json" },
      dependencies: ["setup"],
    },
  ],
});
