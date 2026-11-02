import { describe, expect, it } from "vitest";

import { defaultView, parseWorkspace } from "./tabs";
import { applySavedState, isModified, pickSavedState, savedViewLink } from "./view-state";

describe("saved view state", () => {
  const view = {
    ...defaultView(),
    filters: [{ column: "status", op: "in" as const, values: ["paid"] }],
    sort: [{ column: "id", desc: true }],
    hidden: ["note"],
    frozen: 1,
    widths: { id: 80 },
    page: 4,
  };

  it("keeps filters, sort, columns and page size, not the page", () => {
    const saved = pickSavedState(view);
    expect(saved).not.toHaveProperty("page");
    const patch = applySavedState(JSON.parse(JSON.stringify(saved)), "v1");
    expect(patch).toMatchObject({ filters: view.filters, sort: view.sort, hidden: ["note"], frozen: 1, page: 1, mode: "data", savedView: "v1" });
  });

  it("drops what is malformed", () => {
    const patch = applySavedState({ filters: [{ column: 1 }], sort: "x", hidden: [2, "a"] }, "v2");
    expect(patch).toMatchObject({ filters: [], sort: [], hidden: ["a"] });
  });

  it("notices drift, but not column widths", () => {
    const saved = pickSavedState(view);
    expect(isModified(view, saved)).toBe(false);
    expect(isModified({ ...view, widths: { id: 200 } }, saved)).toBe(false);
    expect(isModified({ ...view, page: 9 }, saved)).toBe(false);
    expect(isModified({ ...view, search: "x" }, saved)).toBe(true);
    expect(isModified({ ...view, hidden: [] }, saved)).toBe(true);
  });

  it("keeps the saved view id across a reload", () => {
    const raw = JSON.stringify({ tabs: [{ kind: "table", name: "a", label: "a", view: { savedView: "v9" } }] });
    expect(parseWorkspace(raw).tabs[0].view.savedView).toBe("v9");
  });

  it("links to the table in that view", () => {
    expect(savedViewLink("https://f.dev", "d1", "public.orders", "v1")).toBe(
      "https://f.dev/data?db=d1&table=public.orders&saved=v1",
    );
  });
});
