import { describe, expect, it } from "vitest";

import {
  mergeSchemas,
  defaultSelection,
  resolveSelection,
  selectionLabel,
  summarizeSchemas,
  toggleSchema,
  type SchemaSummary,
} from "./schemas";

const items = [
  { schema: "billing", kind: "table" },
  { schema: "public", kind: "table" },
  { schema: "public", kind: "view" },
  { schema: "audit", kind: "table" },
  { schema: "audit", kind: "table" },
  { schema: "audit", kind: "object" },
];

describe("schema selection", () => {
  const summary = summarizeSchemas(items);

  it("counts per schema, public first", () => {
    expect(summary).toEqual([
      { name: "public", tables: 1, views: 1 },
      { name: "audit", tables: 2, views: 0 },
      { name: "billing", tables: 1, views: 0 },
    ]);
  });

  it("starts on public, or the biggest schema, or everything", () => {
    expect(defaultSelection(summary)).toEqual({ all: false, schemas: ["public"] });
    const noPublic: SchemaSummary[] = summary.filter((s) => s.name !== "public");
    expect(defaultSelection(noPublic)).toEqual({ all: false, schemas: ["audit"] });
    expect(defaultSelection([{ name: "public", tables: 3, views: 0 }])).toEqual({ all: true });
    expect(defaultSelection([])).toEqual({ all: true });
  });

  it("drops schemas that are gone", () => {
    expect(resolveSelection({ all: false, schemas: ["audit", "gone"] }, summary)).toEqual({ all: false, schemas: ["audit"] });
    expect(resolveSelection({ all: false, schemas: ["gone"] }, summary)).toEqual({ all: false, schemas: ["public"] });
    expect(resolveSelection({ all: true }, summary)).toEqual({ all: true });
    expect(resolveSelection(null, summary)).toEqual({ all: false, schemas: ["public"] });
  });

  it("toggles schemas and never empties the selection", () => {
    const one = { all: false as const, schemas: ["public"] };
    expect(toggleSchema(one, "public", summary)).toBe(one);
    const two = toggleSchema(one, "billing", summary);
    expect(two).toEqual({ all: false, schemas: ["public", "billing"] });
    expect(toggleSchema(two, "audit", summary)).toEqual({ all: true });
    expect(toggleSchema({ all: true }, "audit", summary)).toEqual({ all: false, schemas: ["public", "billing"] });
  });

  it("labels the picker", () => {
    expect(selectionLabel({ all: false, schemas: ["public"] }, summary)).toBe("public");
    expect(selectionLabel({ all: false, schemas: ["public", "audit"] }, summary)).toBe("2 schemas");
    expect(selectionLabel({ all: true }, summary)).toBe("All schemas");
  });
});

describe("mergeSchemas", () => {
  const fromTables = summarizeSchemas([
    { schema: "public", kind: "table" },
    { schema: "sales", kind: "view" },
  ]);

  it("adds schemas without tables, keeping the order", () => {
    const merged = mergeSchemas(fromTables, [
      { name: "staging", tables: 0, views: 0, owner: "app" },
      { name: "public", tables: 1, views: 0, owner: "postgres", comment: "standard", bytes: 8192 },
      { name: "sales", tables: 0, views: 1 },
    ]);
    expect(merged.map((s) => s.name)).toEqual(["public", "sales", "staging"]);
    expect(merged[2]).toEqual({ name: "staging", tables: 0, views: 0, owner: "app" });
    expect(merged[0]).toEqual({ name: "public", tables: 1, views: 0, owner: "postgres", comment: "standard", bytes: 8192 });
  });

  it("keeps the table list's counts", () => {
    const merged = mergeSchemas(fromTables, [{ name: "public", tables: 7, views: 3 }]);
    expect(merged[0].tables).toBe(1);
  });

  it("returns the summary unchanged without a server list", () => {
    expect(mergeSchemas(fromTables, undefined)).toBe(fromTables);
    expect(mergeSchemas(fromTables, [])).toBe(fromTables);
  });
});
