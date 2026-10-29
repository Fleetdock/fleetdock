import { describe, expect, it } from "vitest";

import { sqlCompletion } from "./sql-schema";
import type { ColumnCatalog } from "./types";

const cat: ColumnCatalog = {
  truncated: false,
  tables: [
    { schema: "public", name: "users", kind: "table", columns: [{ name: "id", type: "integer" }, { name: "email", type: "text" }] },
    { schema: "sales", name: "orders", kind: "table", columns: [{ name: "id", type: "bigint" }] },
    { schema: "public", name: "active_users", kind: "view", columns: [{ name: "id", type: "integer" }] },
  ],
};

describe("sqlCompletion", () => {
  it("is undefined until the catalog loads", () => {
    expect(sqlCompletion(undefined, true)).toBeUndefined();
  });

  it("lists MySQL tables at the top level with typed columns", () => {
    const mysql: ColumnCatalog = { truncated: false, tables: cat.tables.map(({ schema: _s, ...t }) => t) };
    const c = sqlCompletion(mysql, false)!;
    expect(c.defaultSchema).toBeUndefined();
    const users = (c.schema as Record<string, { self: { label: string; detail: string }; children: { label: string; detail: string }[] }>).users;
    expect(users.self).toMatchObject({ label: "users", detail: "table" });
    expect(users.children).toEqual([
      { label: "id", type: "property", detail: "integer" },
      { label: "email", type: "property", detail: "text" },
    ]);
  });

  it("nests PostgreSQL tables under schemas and defaults to public", () => {
    const c = sqlCompletion(cat, true)!;
    expect(c.defaultSchema).toBe("public");
    const ns = c.schema as Record<string, { self: { label: string }; children: Record<string, { self: { detail: string } }> }>;
    expect(Object.keys(ns).sort()).toEqual(["public", "sales"]);
    expect(Object.keys(ns.public.children).sort()).toEqual(["active_users", "users"]);
    expect(ns.public.children.active_users.self.detail).toBe("view");
    expect(Object.keys(ns.sales.children)).toEqual(["orders"]);
  });

  it("has no default schema when nothing lives in public", () => {
    const c = sqlCompletion({ truncated: false, tables: [cat.tables[1]] }, true)!;
    expect(c.defaultSchema).toBeUndefined();
  });
});
