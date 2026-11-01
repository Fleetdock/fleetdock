import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { parsePlan, walk, type PlanNode } from ".";

const fixture = (name: string) => readFileSync(join(__dirname, "fixtures", name), "utf8");
const json = (name: string) => JSON.parse(fixture(name));

function labels(root: PlanNode): string[] {
  const out: string[] = [];
  walk(root, (n, d) => out.push(`${"  ".repeat(d)}${n.label}`));
  return out;
}

function all(root: PlanNode): PlanNode[] {
  const out: PlanNode[] = [];
  walk(root, (n) => out.push(n));
  return out;
}

// The fixtures are real EXPLAIN output of the same join + ORDER BY on
// PostgreSQL 16, MySQL 8.4 and MariaDB 11.4.
describe("parsePlan", () => {
  it("reads PostgreSQL plans with nested steps", () => {
    const p = parsePlan({ format: "postgres", analyzed: false, plan: json("postgres.json") });
    expect(p.root.label).toBe("Sort");
    const ls = labels(p.root);
    expect(ls.some((l) => l.includes("on items i"))).toBe(true);
    expect(ls.some((l) => l.includes("on orders o"))).toBe(true);
    expect(p.root.cost).toBeGreaterThan(0);
    expect(p.root.timeMs).toBeUndefined();
  });

  it("reads PostgreSQL analyzed plans with actual figures", () => {
    const p = parsePlan({ format: "postgres", analyzed: true, plan: json("postgres-analyze.json") });
    expect(p.analyzed).toBe(true);
    expect(p.totalTimeMs).toBeGreaterThan(0);
    expect(p.root.actualRows).toBe(2);
    expect(p.root.timeMs).toBeGreaterThan(0);
    expect(p.root.detail).toContain("Sort Key: i.sku");
  });

  it("reads MySQL plans: sort wrapper, then the join in order", () => {
    const p = parsePlan({ format: "mysql", analyzed: false, plan: json("mysql.json") });
    expect(labels(p.root)).toEqual([
      "Query",
      "  Sort (ORDER BY)",
      "    Nested loop join",
      "      Full table scan on i",
      "      Unique index lookup on o",
    ]);
    const sort = p.root.children[0];
    expect(sort.warnings.join(" ")).toMatch(/filesort/);
    const scan = all(p.root).find((n) => n.label === "Full table scan on i")!;
    expect(scan.detail).toContain("possible keys");
    expect(all(p.root).find((n) => n.label === "Unique index lookup on o")!.detail).toContain("key PRIMARY (region, num)");
  });

  it("reads MariaDB plans and analyzed plans", () => {
    const p = parsePlan({ format: "mariadb", analyzed: false, plan: json("mariadb.json") });
    const ls = labels(p.root);
    expect(ls.join("\n")).toContain("Sort by i.sku (filesort)");
    expect(ls.join("\n")).toContain("Unique index lookup on o");
    const a = parsePlan({ format: "mariadb", analyzed: true, plan: json("mariadb-analyze.json") });
    const scan = all(a.root).find((n) => n.label === "Full table scan on i")!;
    expect(scan.actualRows).toBeDefined();
    expect(scan.timeMs).toBeGreaterThan(0);
    expect(a.totalTimeMs).toBeGreaterThan(0);
  });

  it("reads MySQL EXPLAIN ANALYZE trees", () => {
    const p = parsePlan({ format: "mysql-tree", analyzed: true, text: fixture("mysql-tree.txt") });
    expect(labels(p.root)).toEqual([
      "Nested loop inner join",
      "  Sort: i.sku",
      "    Filter: ((i.qty > 1) and (i.o_region is not null) and (i.o_num is not null))",
      "      Table scan on i",
      "  Single-row index lookup on o using PRIMARY (region=i.o_region, num=i.o_num)",
    ]);
    const lookup = p.root.children[1];
    expect(lookup.loops).toBe(2);
    expect(lookup.actualRows).toBe(1);
    expect(lookup.cost).toBe(0.35);
  });

  it("flags big scans and misestimates", () => {
    const p = parsePlan({
      format: "mysql-tree",
      analyzed: true,
      text: "-> Filter: (t.a = 1)  (cost=10 rows=5) (actual time=0.1..9 rows=4000 loops=1)\n    -> Table scan on t  (cost=10 rows=50000) (actual time=0.1..8 rows=50000 loops=1)\n",
    });
    expect(p.root.warnings.join(" ")).toMatch(/off by 800×/);
    expect(p.root.children[0].warnings.join(" ")).toMatch(/reads ~50000 rows/);
  });

  it("rejects unknown shapes", () => {
    expect(() => parsePlan({ format: "postgres", analyzed: false, plan: {} })).toThrow();
    expect(() => parsePlan({ format: "mysql-tree", analyzed: true, text: "" })).toThrow();
  });
});
