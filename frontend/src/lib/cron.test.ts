import { describe, expect, it } from "vitest";

import { describeCron, isCron } from "./cron";

describe("describeCron", () => {
  it.each([
    ["0 2 * * *", "Every day at 02:00 UTC"],
    ["30 14 * * *", "Every day at 14:30 UTC"],
    ["0 * * * *", "Every hour"],
    ["15 * * * *", "Every hour at :15"],
    ["*/10 * * * *", "Every 10 minutes"],
    ["0 */6 * * *", "Every 6 hours"],
    ["0 3 * * 0", "Every Sunday at 03:00 UTC"],
    ["0 3 * * 7", "Every Sunday at 03:00 UTC"],
    ["0 3 * * 1-5", "Every weekday at 03:00 UTC"],
    ["0 4 1 * *", "On the 1st of every month at 04:00 UTC"],
    ["0 4 22 * *", "On the 22nd of every month at 04:00 UTC"],
    ["0 4 11 * *", "On the 11th of every month at 04:00 UTC"],
  ])("%s", (expr, want) => {
    expect(describeCron(expr)).toBe(want);
  });

  it("returns null for what it can't say simply", () => {
    expect(describeCron("0 2 * 1 *")).toBeNull();
    expect(describeCron("0 2,14 * * *")).toBeNull();
    expect(describeCron("61 2 * * *")).toBeNull();
    expect(describeCron("nonsense")).toBeNull();
  });
});

describe("isCron", () => {
  it("needs five fields", () => {
    expect(isCron("0 2 * * *")).toBe(true);
    expect(isCron(" 0 2 * * * ")).toBe(true);
    expect(isCron("0 2 * *")).toBe(false);
  });
});
