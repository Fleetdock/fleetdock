import { describe, expect, it } from "vitest";

import { emptyWorkspace, parseWorkspace, reducer, serializeWorkspace, type Action, type Workspace } from "./tabs";

const run = (actions: Action[], ws: Workspace = emptyWorkspace) => actions.reduce(reducer, ws);
const open = (name: string, preview = false): Action => ({
  type: "open",
  tab: { kind: "table", name, label: name, preview },
});
const ids = (ws: Workspace) => ws.tabs.map((t) => t.id);

describe("tabs reducer", () => {
  it("opens tabs after the active one and activates them", () => {
    const ws = run([open("a"), open("b"), { type: "activate", id: "table:a" }, open("c")]);
    expect(ids(ws)).toEqual(["table:a", "table:c", "table:b"]);
    expect(ws.active).toBe("table:c");
  });

  it("focuses an already open tab instead of duplicating it", () => {
    const ws = run([open("a"), open("b"), open("a")]);
    expect(ids(ws)).toEqual(["table:a", "table:b"]);
    expect(ws.active).toBe("table:a");
  });

  it("replaces the preview tab and pins on a full open", () => {
    let ws = run([open("a"), open("b", true), open("c", true)]);
    expect(ids(ws)).toEqual(["table:a", "table:c"]);
    expect(ws.tabs[1].preview).toBe(true);
    ws = reducer(ws, open("c"));
    expect(ws.tabs[1].preview).toBe(false);
    ws = reducer(ws, open("d", true));
    expect(ids(ws)).toEqual(["table:a", "table:c", "table:d"]);
  });

  it("pins a preview tab when its view is changed by the user", () => {
    let ws = run([open("a", true)]);
    ws = reducer(ws, { type: "update", id: "table:a", view: { page: 2 }, pin: true });
    expect(ws.tabs[0].preview).toBe(false);
    expect(ws.tabs[0].view.page).toBe(2);
  });

  it("applies a view on open and resets the page", () => {
    let ws = run([open("a"), { type: "update", id: "table:a", view: { page: 4 } }]);
    ws = reducer(ws, {
      type: "open",
      tab: { kind: "table", name: "a", label: "a", view: { filters: [{ column: "id", op: "eq", value: "7" }] } },
    });
    expect(ws.tabs[0].view.page).toBe(1);
    expect(ws.tabs[0].view.filters).toHaveLength(1);
  });

  it("activates the right neighbour on close, else the left one", () => {
    const base = run([open("a"), open("b"), open("c"), { type: "activate", id: "table:b" }]);
    expect(reducer(base, { type: "close", id: "table:b" }).active).toBe("table:c");
    const last = reducer(base, { type: "activate", id: "table:c" });
    expect(reducer(last, { type: "close", id: "table:c" }).active).toBe("table:b");
    expect(reducer(base, { type: "close", id: "table:a" }).active).toBe("table:b");
  });

  it("closes others, to the right, and all", () => {
    const base = run([open("a"), open("b"), open("c")]);
    expect(ids(reducer(base, { type: "closeOthers", id: "table:b" }))).toEqual(["table:b"]);
    const right = reducer(base, { type: "closeRight", id: "table:a" });
    expect(ids(right)).toEqual(["table:a"]);
    expect(right.active).toBe("table:a");
    expect(reducer(base, { type: "closeAll" })).toMatchObject({ tabs: [], active: null });
  });

  it("moves and cycles tabs", () => {
    let ws = run([open("a"), open("b"), open("c")]);
    ws = reducer(ws, { type: "move", id: "table:c", to: 0 });
    expect(ids(ws)).toEqual(["table:c", "table:a", "table:b"]);
    ws = reducer(ws, { type: "activate", id: "table:b" });
    expect(reducer(ws, { type: "cycle", delta: 1 }).active).toBe("table:c");
    expect(reducer(ws, { type: "cycle", delta: -1 }).active).toBe("table:a");
  });

  it("numbers query tabs", () => {
    const ws = run([{ type: "newQuery" }, { type: "newQuery", sql: "SELECT 1" }]);
    expect(ws.tabs.map((t) => t.label)).toEqual(["Query 1", "Query 2"]);
    expect(ws.tabs[1].sql).toBe("SELECT 1");
  });

  it("follows a rename, merging into an already open tab", () => {
    let ws = run([open("a"), open("b")]);
    ws = reducer(ws, { type: "rename", id: "table:b", name: "z", label: "z" });
    expect(ids(ws)).toEqual(["table:a", "table:z"]);
    expect(ws.active).toBe("table:z");
    ws = reducer(ws, { type: "rename", id: "table:z", name: "a", label: "a" });
    expect(ids(ws)).toEqual(["table:a"]);
    expect(ws.active).toBe("table:a");
  });
});

describe("workspace persistence", () => {
  it("round-trips", () => {
    const ws = run([open("public.a"), { type: "newQuery", sql: "SELECT 1" }]);
    expect(parseWorkspace(serializeWorkspace(ws))).toEqual(ws);
  });

  it("drops malformed data", () => {
    expect(parseWorkspace("not json")).toEqual(emptyWorkspace);
    expect(parseWorkspace(null)).toEqual(emptyWorkspace);
    const ws = parseWorkspace(
      JSON.stringify({
        active: "table:gone",
        tabs: [
          { kind: "table", name: "a", label: "a", view: { pageSize: 7, page: -2, filters: [{ column: "x", op: "drop" }] } },
          { kind: "table", name: "a", label: "dup" },
          { kind: "bogus", name: "b", label: "b" },
        ],
      }),
    );
    expect(ids(ws)).toEqual(["table:a"]);
    expect(ws.active).toBe("table:a");
    expect(ws.tabs[0].view).toMatchObject({ pageSize: 100, page: 1, filters: [] });
  });
});
