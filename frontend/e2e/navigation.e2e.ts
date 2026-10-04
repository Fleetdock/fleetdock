import { expect, test } from "@playwright/test";

const SECTIONS = [
  ["Overview", "/dashboard", "Overview"],
  ["Servers", "/servers", "Servers"],
  ["Databases", "/databases", "Databases"],
  ["Data browser", "/data", "Data browser"],
  ["Backups", "/backups", "Backup history"],
  ["Activity", "/activity", "Activity"],
  ["Access", "/access/users", "Users"],
  ["Settings", "/settings/notifications", "Notifications"],
] as const;

test("every section opens from the sidebar", async ({ page }) => {
  const errors: string[] = [];
  page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
  await page.goto("/dashboard");
  const nav = page.getByRole("navigation", { name: "Main" });
  for (const [label, path, heading] of SECTIONS) {
    await nav.getByRole("link", { name: label }).click();
    await expect(page).toHaveURL(new RegExp(`${path}$`));
    await expect(page.getByRole("heading", { level: 1, name: heading })).toBeVisible();
  }
  expect(errors, errors.join("\n")).toEqual([]);
});

test("old addresses still work", async ({ page }) => {
  for (const [from, to] of [
    ["/operations", "/activity"],
    ["/destinations", "/backups/storage"],
    ["/users", "/access/users"],
    ["/profile", "/settings/profile"],
    ["/instances", "/databases"],
  ]) {
    await page.goto(from);
    await expect(page).toHaveURL(new RegExp(`${to}$`));
  }
});

test("phone layout: drawer navigation, no sideways scrolling", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/access/users");
  await page.getByRole("button", { name: "Open navigation" }).click();
  await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "Activity" }).click();
  await expect(page).toHaveURL(/\/activity$/);
  await expect(page.getByRole("button", { name: "Open navigation" })).toBeVisible();
  for (const path of ["/dashboard", "/databases", "/backups", "/activity", "/access/users"]) {
    await page.goto(path);
    await page.waitForLoadState("networkidle");
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow, `${path} scrolls sideways`).toBeLessThanOrEqual(0);
  }
});

test("command palette finds pages and opens them with the keyboard", async ({ page }) => {
  await page.goto("/dashboard");
  await expect(page.getByRole("heading", { level: 1, name: "Overview" })).toBeVisible();
  await page.keyboard.press("ControlOrMeta+k");
  const dialog = page.getByRole("dialog", { name: "Go to…" });
  await expect(dialog).toBeVisible();
  await page.keyboard.type("storage");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/backups\/storage$/);
  await expect(dialog).toBeHidden();
  // "g" then "a" jumps to Activity; "?" shows the shortcuts.
  await page.keyboard.press("g");
  await page.keyboard.press("a");
  await expect(page).toHaveURL(/\/activity$/);
  await page.keyboard.press("?");
  await expect(page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeHidden();
});
