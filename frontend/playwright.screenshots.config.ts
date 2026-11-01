import { defineConfig } from "@playwright/test";

/**
 * Captures the product screenshots in docs/screenshots/{light,dark}/ from a
 * running, seeded Fleetdock. Normally run by scripts/screenshots.sh:
 *
 *   FLEETDOCK_SHOTS_URL=http://127.0.0.1:18080 npm run screenshots
 *
 * Kept apart from the e2e config so `npm run e2e` never picks it up.
 */
export default defineConfig({
  testDir: "e2e/screenshots",
  testMatch: /.*\.capture\.ts/,
  timeout: 120_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1,
  reporter: [["list"]],
  use: {
    baseURL: process.env.FLEETDOCK_SHOTS_URL ?? "http://127.0.0.1:18080",
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 2,
    colorScheme: "light",
    locale: "en-US",
    timezoneId: "UTC",
  },
  projects: [
    { name: "light", use: { colorScheme: "light" } },
    { name: "dark", use: { colorScheme: "dark" } },
  ],
});
