import { describe, expect, it } from "vitest";

import { changeLabel } from "./schema-changes";

describe("changeLabel", () => {
  it("names table-level objects with their table", () => {
    expect(changeLabel({ kind: "column", op: "added", table: "items", name: "qty" })).toBe("column items.qty");
    expect(changeLabel({ kind: "index", op: "removed", schema: "public", table: "items", name: "ix" })).toBe("index items.ix");
    expect(changeLabel({ kind: "trigger", op: "changed", schema: "audit", table: "log", name: "t" })).toBe("trigger audit.log.t");
  });
  it("names tables and standalone objects", () => {
    expect(changeLabel({ kind: "table", op: "added", schema: "public", table: "orders" })).toBe("table orders");
    expect(changeLabel({ kind: "options", op: "changed", table: "orders" })).toBe("table options orders");
    expect(changeLabel({ kind: "routine", op: "added", schema: "app", name: "f(integer)" })).toBe("routine app.f(integer)");
    expect(changeLabel({ kind: "view", op: "removed", schema: "public", name: "v" })).toBe("view v");
  });
});
