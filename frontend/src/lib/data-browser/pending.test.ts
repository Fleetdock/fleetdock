import { describe, expect, it } from "vitest";

import type { BrowseColumn } from "../types";
import { changeCount, emptyPending, keyOf, pendingReducer, rowIdOf, toMutations, type PendingAction } from "./pending";

const columns: BrowseColumn[] = [
  { name: "id", type: "bigint", nullable: false, has_default: true },
  { name: "email", type: "text", nullable: false, has_default: false },
  { name: "note", type: "text", nullable: true, has_default: false },
];
const key = ["id"];
const row = ["1", "a@x", null];
const rowId = rowIdOf(key, keyOf(columns, key, row));
const run = (actions: PendingAction[]) => actions.reduce(pendingReducer, emptyPending);
const set = (column: string, value: string | null, original: string | null): PendingAction => ({
  type: "set",
  rowId,
  key: { id: "1" },
  column,
  value,
  original,
});

describe("pending edits", () => {
  it("identifies rows by their key values", () => {
    expect(keyOf(columns, key, row)).toEqual({ id: "1" });
    expect(rowId).toBe('["1"]');
  });

  it("records changed cells and reverts when edited back", () => {
    let p = run([set("email", "b@x", "a@x"), set("note", "hi", null)]);
    expect(p.updates[rowId].values).toEqual({ email: "b@x", note: "hi" });
    expect(changeCount(p)).toBe(1);
    p = pendingReducer(p, set("email", "a@x", "a@x"));
    expect(p.updates[rowId].values).toEqual({ note: "hi" });
    p = pendingReducer(p, { type: "revertCell", rowId, column: "note" });
    expect(p.updates).toEqual({});
  });

  it("starts new rows with defaults left out and NULL/empty otherwise", () => {
    const p = run([{ type: "insert", columns }]);
    expect(p.inserts[0].values).toEqual({ email: "", note: null });
    const q = pendingReducer(p, { type: "setNew", id: p.inserts[0].id, column: "id", value: "9" });
    expect(q.inserts[0].values.id).toBe("9");
    const r = pendingReducer(q, { type: "setNew", id: p.inserts[0].id, column: "id", value: undefined });
    expect("id" in r.inserts[0].values).toBe(false);
  });

  it("duplicates a row without its key", () => {
    const p = run([{ type: "duplicate", columns, row, key }]);
    expect(p.inserts[0].values).toEqual({ email: "a@x", note: null });
  });

  it("lets a delete supersede an update", () => {
    const p = run([set("email", "b@x", "a@x"), { type: "delete", rows: [{ rowId, key: { id: "1" } }] }]);
    expect(p.updates).toEqual({});
    expect(p.deletes[rowId]).toEqual({ id: "1" });
    expect(pendingReducer(p, { type: "undelete", rowId }).deletes).toEqual({});
  });

  it("orders mutations deletes, updates, then inserts oldest first", () => {
    const p = run([
      { type: "insert", columns },
      { type: "insert", columns },
      set("email", "b@x", "a@x"),
      { type: "delete", rows: [{ rowId: '["2"]', key: { id: "2" } }] },
    ]);
    expect(toMutations(p).map((m) => `${m.kind}:${m.id}`)).toEqual([
      'delete:["2"]',
      'update:["1"]',
      "insert:new-1",
      "insert:new-2",
    ]);
  });

  it("keeps failed changes with their error after a save", () => {
    let p = run([{ type: "insert", columns }, set("email", "b@x", "a@x")]);
    p = pendingReducer(p, {
      type: "results",
      results: [
        { kind: "update", id: rowId, ok: true },
        { kind: "insert", id: "new-1", ok: false, error: "email must not be empty" },
      ],
    });
    expect(p.updates).toEqual({});
    expect(p.inserts).toHaveLength(1);
    expect(p.errors).toEqual({ "new-1": "email must not be empty" });
    expect(changeCount(pendingReducer(p, { type: "discard" }))).toBe(0);
  });
});
