import { expect, test } from "@playwright/test";

test("a destructive action asks first and works from the keyboard", async ({ page }) => {
  const name = `e2e-${Date.now()}`;
  await page.goto("/access/tokens");
  await page.getByRole("button", { name: "New token" }).click();
  const create = page.getByRole("dialog", { name: "New API token" });
  await create.getByLabel("Name").fill(name);
  await create.getByRole("button", { name: "Create token" }).click();
  await page.getByRole("dialog", { name: "Token created" }).getByRole("button", { name: "Done" }).click();

  const row = page.getByRole("row", { name: new RegExp(name) });
  await row.getByRole("button", { name: `Revoke ${name}` }).click();
  const confirm = page.getByRole("dialog", { name: `Revoke "${name}"?` });
  await expect(confirm).toBeVisible();
  // Esc cancels and returns focus to the button that opened it.
  await page.keyboard.press("Escape");
  await expect(confirm).toBeHidden();
  await expect(row.getByRole("button", { name: `Revoke ${name}` })).toBeFocused();

  await page.keyboard.press("Enter");
  await expect(confirm).toBeVisible();
  await confirm.getByRole("button", { name: "Revoke token" }).focus();
  await page.keyboard.press("Enter");
  await expect(confirm).toBeHidden();
  await expect(row.getByText("revoked")).toBeVisible();
});

test("the overview walks a new install through setup", async ({ page }) => {
  const overview = page.waitForResponse((r) => r.url().endsWith("/v1/overview"));
  await page.goto("/dashboard");
  const { setup: steps } = await (await overview).json();
  test.skip(steps.instances > 0 && steps.destinations > 0 && steps.schedules > 0 && steps.channels > 0, "setup already complete");
  const setup = page.getByRole("region", { name: "Get started" });
  await expect(setup).toBeVisible();
  if (steps.instances === 0) {
    // First step opens the connect wizard straight away.
    await setup.getByRole("link", { name: "Connect", exact: true }).click();
    await expect(page.getByRole("dialog", { name: "Connect a database server" })).toBeVisible();
    return;
  }
  await expect(setup.getByRole("listitem").filter({ hasText: "Connect a database server" }).getByLabel("Done")).toBeVisible();
  if (steps.destinations === 0) {
    await setup.getByRole("link", { name: "Add storage" }).click();
    await expect(page).toHaveURL(/\/backups\/storage$/);
  }
});
