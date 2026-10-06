import { execFileSync } from "node:child_process";

import { expect, test } from "@playwright/test";

/*
 * Automatic discovery against a real PostgreSQL. Needs a throwaway server
 * Fleetdock can reach, and (for the missing/reappear part) a docker container
 * name to run psql in:
 *
 *   FLEETDOCK_E2E_PG_HOST=fdverify-target-pg FLEETDOCK_E2E_PG_PASSWORD=… \
 *   FLEETDOCK_E2E_PG_CONTAINER=fdverify-target-pg npm run e2e
 */
const host = process.env.FLEETDOCK_E2E_PG_HOST;
const password = process.env.FLEETDOCK_E2E_PG_PASSWORD ?? "";
const container = process.env.FLEETDOCK_E2E_PG_CONTAINER;

function psql(sql: string) {
  execFileSync("docker", ["exec", container!, "psql", "-U", "postgres", "-c", sql], { stdio: "pipe" });
}

test.describe.configure({ mode: "serial" });

test("a connected server's databases appear and disappear on their own", async ({ page }) => {
  test.skip(!host, "FLEETDOCK_E2E_PG_HOST not set");
  test.setTimeout(9 * 60_000);
  const server = `e2e-pg-${Date.now()}`;
  const db = `e2e_${Date.now()}`;
  if (container) psql(`CREATE DATABASE ${db}`);

  await page.goto("/databases");
  await page.getByRole("button", { name: "Connect database server" }).first().click();
  // The wizard is one dialog whose title follows the step.
  const wizard = page.getByRole("dialog");
  await expect(wizard).toHaveAccessibleName("Connect a database server");
  await wizard.getByRole("button", { name: /Connect to one anywhere/ }).click();
  await expect(wizard).toHaveAccessibleName("Connect to one anywhere");
  await wizard.getByLabel("Engine").selectOption("postgres");
  await wizard.getByLabel("Name").fill(server);
  await wizard.getByLabel("Host").fill(host!);
  await wizard.getByLabel("Admin user").fill("postgres");
  await wizard.getByLabel("Password").fill(password);
  await wizard.getByRole("button", { name: "Connect", exact: true }).click();

  // No "test" or "import" clicks: the databases just show up.
  const done = page.getByRole("dialog", { name: "Database server added" });
  await expect(done).toBeVisible();
  await expect(done.getByText(/Found \d+ databases?/)).toBeVisible({ timeout: 90_000 });
  await done.getByRole("link", { name: `Open ${server}` }).click();
  const target = container ? db : "postgres";
  await expect(page.getByRole("link", { name: target, exact: true })).toBeVisible({ timeout: 90_000 });
  if (!container) return;

  // Dropped outside Fleetdock → "not found on server" after ~3 probes.
  psql(`DROP DATABASE ${db}`);
  const row = page.getByRole("row", { name: new RegExp(`^${db}\\b`) });
  await expect(row.getByText("not found on server")).toBeVisible({ timeout: 5 * 60_000 });

  // Recreated → active again on the next check ("Check now" skips the wait).
  psql(`CREATE DATABASE ${db}`);
  await page.getByRole("button", { name: "Check now" }).click();
  await expect(row.getByText("active")).toBeVisible({ timeout: 30_000 });
});
