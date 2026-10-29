import { describe, expect, it } from "vitest";

import { mergeResults } from "./fleet";
import type { FleetTarget, QueryResult } from "./types";

const res = (columns: string[], rows: (string | null)[][], truncated = false): QueryResult => ({
  columns,
  rows,
  row_count: rows.length,
  truncated,
  rows_affected: 0,
  read_only: true,
  duration_ms: 1,
});

const target = (name: string, results?: QueryResult[]): FleetTarget => ({
  position: 0,
  database_name: name,
  instance_name: `srv-${name}`,
  engine: "postgres",
  status: results ? "succeeded" : "failed",
  results: results ? { results } : undefined,
});

describe("mergeResults", () => {
  it("unions columns and keeps each database's rows", () => {
    const m = mergeResults([
      target("a", [res([], []), res(["id", "name"], [["1", "x"]])]),
      target("b", [res(["id", "extra"], [["2", null], ["3", "y"]], true)]),
      target("c"),
    ]);
    expect(m.columns).toEqual(["id", "name", "extra"]);
    expect(m.rows).toEqual([
      { database: "a", server: "srv-a", cells: ["1", "x", null] },
      { database: "b", server: "srv-b", cells: ["2", null, null] },
      { database: "b", server: "srv-b", cells: ["3", null, "y"] },
    ]);
    expect(m.trimmed).toEqual(["b"]);
  });
});
