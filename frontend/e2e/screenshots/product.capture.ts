import { mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";

import { expect, test, type Page } from "@playwright/test";

/**
 * Product screenshots for the README, docs and fleetdock.dev. Expects the
 * demo stack from deploy/demo with scripts/demo/seed.mjs applied. Each shot is
 * written to docs/screenshots/<light|dark>/<name>.png at 1440×900 @2x.
 */

const EMAIL = process.env.FLEETDOCK_SHOTS_EMAIL ?? "admin@demo.fleetdock.dev";
const PASSWORD = process.env.FLEETDOCK_SHOTS_PASSWORD ?? "fleetdock-demo-password";
// Run from frontend/ (npm run screenshots); images go to the repo's docs/.
const OUT = resolve(process.cwd(), "../docs/screenshots");
// ONLY=overview,backups captures a subset.
const ONLY = (process.env.ONLY ?? "").split(",").filter(Boolean);

type Ids = { shop: string; crm: string; prodPostgres: string; fleetRun?: string; server?: string };

async function api<T>(page: Page, path: string): Promise<T> {
  const res = await page.request.get(path);
  expect(res.ok(), `${path} → ${res.status()}`).toBeTruthy();
  return (await res.json()) as T;
}

async function resolveIds(page: Page): Promise<Ids> {
  type List<T> = { items?: T[] } | T[];
  const items = <T,>(l: List<T>): T[] => (Array.isArray(l) ? l : (l.items ?? []));
  const dbs = items(await api<List<{ id: string; name: string }>>(page, "/v1/databases?page_size=200"));
  const instances = items(await api<List<{ id: string; name: string }>>(page, "/v1/instances?page_size=200"));
  const runs = items(await api<List<{ id: string }>>(page, "/v1/fleet-runs"));
  const servers = items(await api<List<{ id: string; name: string }>>(page, "/v1/servers"));
  const find = <T extends { name: string; id: string }>(l: T[], name: string) => {
    const hit = l.find((x) => x.name === name);
    if (!hit) throw new Error(`demo data missing "${name}"; run scripts/demo/seed.mjs`);
    return hit.id;
  };
  return { shop: find(dbs, "shop"), crm: find(dbs, "crm"), prodPostgres: find(instances, "prod-postgres"), fleetRun: runs[0]?.id, server: servers[0]?.id };
}

// settle waits for data to load and animations to finish.
async function settle(page: Page) {
  await page.waitForLoadState("networkidle").catch(() => {});
  await expect(page.locator('[aria-busy="true"], .skeleton')).toHaveCount(0, { timeout: 15_000 }).catch(() => {});
  await page.waitForTimeout(400);
}

async function draft(page: Page, dbId: string, sql: string) {
  await page.evaluate(([id, s]) => localStorage.setItem(`fleetdock:data-browser:draft:${id}`, s), [dbId, sql]);
}

// Feature close-ups use a smaller window (same 16:10 shape) so the UI reads
// larger when the image is shown at half width on the website.
type Shot = { name: string; closeUp?: boolean; run: (page: Page, ids: Ids) => Promise<void> };
const CLOSE_UP = { width: 1200, height: 750 };

const SHOTS: Shot[] = [
  { name: "overview", run: async (p) => void (await p.goto("/dashboard")) },
  { name: "servers", run: async (p) => void (await p.goto("/servers")) },
  {
    name: "server-metrics",
    closeUp: true,
    run: async (p, ids) => {
      test.skip(!ids.server, "no agent-connected server in the demo");
      await p.goto(`/servers/${ids.server}`);
    },
  },
  { name: "databases", run: async (p) => void (await p.goto("/databases")) },
  { name: "database-detail", run: async (p, ids) => void (await p.goto(`/databases/${ids.shop}`)) },
  {
    name: "schema-history",
    closeUp: true,
    run: async (p, ids) => {
      await p.goto(`/databases/${ids.shop}?tab=schema`);
      await settle(p);
      const head = p.locator(".schema-snapshot-head").first();
      if ((await head.getAttribute("aria-expanded")) !== "true") await head.click();
      // Put the Backups / Access / Schema tabs at the top of the shot.
      await p.getByRole("tab", { name: "Schema" }).evaluate((el) => el.scrollIntoView({ block: "start" }));
      await p.evaluate(() => window.scrollBy(0, -88));
    },
  },
  {
    name: "data-browser",
    closeUp: true,
    run: async (p, ids) => {
      await p.goto(`/data?db=${ids.shop}&table=public.customers`);
      await settle(p);
      await p.locator(".dbx-cell").nth(3).click();
    },
  },
  {
    name: "data-browser-history",
    closeUp: true,
    run: async (p, ids) => {
      await p.goto(`/data?db=${ids.shop}&table=public.products`);
      await settle(p);
      await p.getByRole("button", { name: "Toggle change history" }).click();
    },
  },
  {
    name: "sql-console",
    closeUp: true,
    run: async (p, ids) => {
      await p.goto("/dashboard");
      await draft(p, ids.shop, "-- Raise audio prices by 10%\nUPDATE products\n   SET price_cents = round(price_cents * 1.1)\n WHERE category = 'audio'\n   AND active;");
      await p.goto(`/data?db=${ids.shop}&query=draft`);
      await settle(p);
      await p.getByRole("button", { name: /Preview/ }).first().click();
    },
  },
  {
    name: "explain-plan",
    closeUp: true,
    run: async (p, ids) => {
      await p.goto("/dashboard");
      await draft(
        p,
        ids.shop,
        "SELECT c.country, count(*) AS orders, sum(o.total_cents) / 100.0 AS revenue\n  FROM orders o\n  JOIN customers c ON c.id = o.customer_id\n WHERE o.placed_at > now() - interval '7 days'\n GROUP BY c.country\n ORDER BY revenue DESC;",
      );
      await p.goto(`/data?db=${ids.shop}&query=draft`);
      await settle(p);
      await p.getByRole("button", { name: /^Analyze/ }).first().click().catch(async () => {
        await p.getByRole("button", { name: /Explain/ }).first().click();
      });
    },
  },
  {
    name: "fleet-query",
    closeUp: true,
    run: async (p, ids) => {
      test.skip(!ids.fleetRun, "no fleet run seeded");
      await p.goto(`/fleet/${ids.fleetRun}`);
    },
  },
  { name: "backups", closeUp: true, run: async (p) => void (await p.goto("/backups")) },
  { name: "backup-schedules", run: async (p) => void (await p.goto("/backups/schedules")) },
  { name: "activity", run: async (p) => void (await p.goto("/activity")) },
  {
    name: "instance-detail",
    closeUp: true,
    run: async (p, ids) => {
      await p.goto(`/instances/${ids.prodPostgres}`);
      await settle(p);
      await p.getByRole("tab", { name: /Monitoring/ }).click();
      await settle(p);
      await p.getByRole("tab", { name: /Top queries/ }).click();
      await settle(p);
      // Put the server's tabs at the top of the shot.
      await p.getByRole("tab", { name: /Monitoring/ }).evaluate((el) => el.scrollIntoView({ block: "start" }));
      await p.evaluate(() => window.scrollBy(0, -88));
    },
  },
  {
    name: "command-palette",
    run: async (p) => {
      await p.goto("/databases");
      await settle(p);
      await p.keyboard.press("ControlOrMeta+k");
      await p.keyboard.type("orders", { delay: 40 });
    },
  },
];


// Sign in once per theme (the login endpoint is rate-limited) and reuse the
// session for every shot.
const STATE = resolve(process.cwd(), "test-results/screenshots-auth.json");
let ids: Ids;

test.beforeAll(async ({ browser }) => {
  const ctx = await browser.newContext({ storageState: { cookies: [], origins: [] } });
  const page = await ctx.newPage();
  const res = await page.request.post("/v1/auth/login", { data: { email: EMAIL, password: PASSWORD } });
  expect(res.ok(), `demo login → ${res.status()}`).toBeTruthy();
  ids = await resolveIds(page);
  mkdirSync(dirname(STATE), { recursive: true });
  await ctx.storageState({ path: STATE });
  await ctx.close();
});

test.use({ storageState: STATE });

test.beforeEach(async ({ page }, info) => {
  const theme = info.project.name === "dark" ? "dark" : "light";
  await page.addInitScript((t) => {
    try {
      localStorage.setItem("theme", t);
      // Keep the setup checklist out of the shots: the demo is fully set up.
      localStorage.setItem("fleetdock.setup-dismissed", "1");
    } catch {
      /* ignore */
    }
  }, theme);
});

for (const shot of SHOTS) {
  test(shot.name, async ({ page }, info) => {
    test.skip(ONLY.length > 0 && !ONLY.includes(shot.name), "not selected");
    if (shot.closeUp) await page.setViewportSize(CLOSE_UP);
    await shot.run(page, ids);
    await settle(page);
    // Hide transient UI: toasts and the mouse-hover state.
    await page.addStyleTag({ content: ".toasts{display:none!important} *{caret-color:transparent!important}" });
    await page.mouse.move(0, 899);
    const file = resolve(OUT, info.project.name, `${shot.name}.png`);
    mkdirSync(dirname(file), { recursive: true });
    await page.screenshot({ path: file, animations: "disabled" });
  });
}
