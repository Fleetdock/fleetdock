import { describe, expect, it } from "vitest";

import type { RowChange } from "../types";
import { changedColumns, displayValue, groupByBatch, keyLabel, sameValue } from "./history";

const base: RowChange = {
  id: "c1",
  database_id: "d",
  table: "items",
  op: "update",
  key: { id: "7" },
  before: { id: "7", note: null, qty: "2" },
  after: { id: "7", note: "hi", qty: "2" },
  columns: ["id", "note", "qty"],
  undoable: true,
  source: "editor",
  created_at: "2026-10-29T10:00:00Z",
};

describe("history helpers", () => {
  it("renders values", () => {
    expect(displayValue(null)).toBe("NULL");
    expect(displayValue("x")).toBe("x");
    expect(displayValue({ b64: "AP8B" })).toBe("(binary, 3 bytes)");
    expect(displayValue({ b64: "AA==" })).toBe("(binary, 1 byte)");
  });

  it("compares values", () => {
    expect(sameValue(null, null)).toBe(true);
    expect(sameValue(null, "")).toBe(false);
    expect(sameValue({ b64: "AA==" }, { b64: "AA==" })).toBe(true);
    expect(sameValue({ b64: "AA==" }, "AA==")).toBe(false);
  });

  it("lists only changed columns of an update", () => {
    expect(changedColumns(base)).toEqual([{ column: "note", before: null, after: "hi" }]);
  });

  it("lists every column of a delete as before-only", () => {
    const del: RowChange = { ...base, op: "delete", after: null };
    expect(changedColumns(del).map((d) => [d.column, d.before, d.after])).toEqual([
      ["id", "7", undefined],
      ["note", null, undefined],
      ["qty", "2", undefined],
    ]);
  });

  it("labels keys", () => {
    expect(keyLabel({ ...base, key: { region: "eu", num: "1" } })).toBe("region = eu, num = 1");
  });

  it("groups one save's changes", () => {
    const groups = groupByBatch([
      { ...base, id: "a", batch_id: "b1" },
      { ...base, id: "b", batch_id: "b1", undone_at: "2026-10-29T10:01:00Z" },
      { ...base, id: "c" },
      { ...base, id: "d", batch_id: "b2", undoable: false },
    ]);
    expect(groups.map((g) => [g.id, g.changes.length, g.undoable])).toEqual([
      ["b1", 2, 1],
      ["c", 1, 1],
      ["b2", 1, 0],
    ]);
  });
});
