import { describe, expect, it } from "vitest";

import { compareValues, resultColumns, sortRowIndexes, withNumericTypes } from "./results";

describe("query results", () => {
  it("makes column names unique", () => {
    expect(resultColumns(["id", "name", "id", "id"]).map((c) => c.name)).toEqual(["id", "name", "id (2)", "id (3)"]);
  });

  it("compares numbers, text and NULL", () => {
    expect(compareValues("9", "10")).toBeLessThan(0);
    expect(compareValues("item2", "item10")).toBeLessThan(0);
    expect(compareValues(null, "a")).toBeGreaterThan(0);
  });

  it("sorts by several keys and keeps NULL last", () => {
    const cols = resultColumns(["a", "b"]);
    const rows = [
      ["2", "x"],
      [null, "y"],
      ["10", "z"],
      ["2", "a"],
    ];
    expect(sortRowIndexes(rows, cols, [{ column: "a", desc: false }, { column: "b", desc: false }])).toEqual([3, 0, 2, 1]);
    expect(sortRowIndexes(rows, cols, [{ column: "a", desc: true }])).toEqual([2, 0, 3, 1]);
    expect(sortRowIndexes(rows, cols, [])).toEqual([0, 1, 2, 3]);
  });

  it("spots numeric columns", () => {
    const cols = withNumericTypes(resultColumns(["n", "t", "e"]), [
      ["1", "a", null],
      ["-2.5", "3", null],
    ]);
    expect(cols.map((c) => c.type)).toEqual(["numeric", "", ""]);
  });
});
