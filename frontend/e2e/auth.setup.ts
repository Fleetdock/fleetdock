import { expect, test as setup } from "@playwright/test";

setup("sign in", async ({ page }) => {
  setup.skip(!process.env.FLEETDOCK_E2E_URL, "FLEETDOCK_E2E_URL not set");
  await page.goto("/login");
  await page.getByLabel("Email").fill(process.env.FLEETDOCK_E2E_EMAIL ?? "admin@example.com");
  await page.getByLabel("Password").fill(process.env.FLEETDOCK_E2E_PASSWORD ?? "");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  await page.context().storageState({ path: "e2e/.auth/admin.json" });
});
