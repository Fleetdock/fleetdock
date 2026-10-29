import { describe, expect, it } from "vitest";

import type { ForeignKey } from "../types";
import { describeFilters, fkByColumn, followFilters, referencingFilters } from "./fk";

const composite: ForeignKey = {
  name: "fk_items_order",
  table: "items",
  columns: ["o_region", "o_num"],
  ref_table: "orders",
  ref_columns: ["region", "num"],
  on_delete: "NO ACTION",
  on_update: "NO ACTION",
};

describe("followFilters", () => {
  it("filters the referenced table on every key column", () => {
    expect(followFilters(composite, { o_region: "eu", o_num: "7", id: "1" })).toEqual([
      { column: "region", op: "eq", value: "eu" },
      { column: "num", op: "eq", value: "7" },
    ]);
  });

  it("follows nothing when a key column is NULL", () => {
    expect(followFilters(composite, { o_region: "eu", o_num: null })).toBeNull();
  });
});

describe("referencingFilters", () => {
  it("filters the referencing table by this row's referenced columns", () => {
    expect(referencingFilters(composite, { region: "eu", num: "7", note: null })).toEqual([
      { column: "o_region", op: "eq", value: "eu" },
      { column: "o_num", op: "eq", value: "7" },
    ]);
  });

  it("is null when the referenced columns are not in the row", () => {
    expect(referencingFilters(composite, { region: "eu" })).toBeNull();
  });
});

describe("fkByColumn", () => {
  it("maps every column of a composite key, first key winning", () => {
    const single: ForeignKey = { ...composite, name: "fk_region", columns: ["o_region"], ref_columns: ["code"], ref_table: "regions" };
    const m = fkByColumn([composite, single]);
    expect(m.o_region.name).toBe("fk_items_order");
    expect(m.o_num.name).toBe("fk_items_order");
  });

  it("skips malformed keys", () => {
    expect(fkByColumn([{ ...composite, ref_columns: ["region"] }])).toEqual({});
  });
});

it("describes filters", () => {
  expect(describeFilters([{ column: "a", op: "eq", value: "1" }, { column: "b", op: "eq", value: "x" }])).toBe("a = 1, b = x");
});
