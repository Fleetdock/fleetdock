import { BIG_SCAN_ROWS, misestimate, num, type Plan, type PlanNode } from "./types";

// PostgreSQL EXPLAIN (FORMAT JSON): [{ "Plan": {…, "Plans": [...]}, "Execution Time": … }].

type PgNode = Record<string, unknown> & { Plans?: PgNode[] };

function label(n: PgNode): string {
  const type = String(n["Node Type"] ?? "Step");
  const parts = [type];
  if (n["Join Type"] && /Join|Nested Loop/.test(type)) parts[0] = `${n["Join Type"]} ${type}`;
  if (n["Index Name"]) parts.push(`using ${n["Index Name"]}`);
  if (n["Relation Name"]) {
    const alias = n["Alias"] && n["Alias"] !== n["Relation Name"] ? ` ${n["Alias"]}` : "";
    parts.push(`on ${n["Relation Name"]}${alias}`);
  } else if (n["CTE Name"]) {
    parts.push(`on ${n["CTE Name"]}`);
  }
  return parts.join(" ");
}

const DETAIL_KEYS = ["Index Cond", "Recheck Cond", "Hash Cond", "Merge Cond", "Join Filter", "Filter", "Sort Key", "Group Key"];

function detail(n: PgNode): string | undefined {
  const out: string[] = [];
  for (const k of DETAIL_KEYS) {
    const v = n[k];
    if (v === undefined) continue;
    out.push(`${k}: ${Array.isArray(v) ? v.join(", ") : String(v)}`);
  }
  return out.length ? out.join(" · ") : undefined;
}

function convert(n: PgNode, analyzed: boolean): PlanNode {
  const loops = num(n["Actual Loops"]);
  const actualRows = num(n["Actual Rows"]);
  const estRows = num(n["Plan Rows"]);
  const total = num(n["Actual Total Time"]);
  const warnings: string[] = [];
  const type = String(n["Node Type"] ?? "");
  if (type === "Seq Scan") {
    const removed = num(n["Rows Removed by Filter"]) ?? 0;
    const scanned = analyzed && actualRows !== undefined ? (actualRows + removed) * (loops ?? 1) : (estRows ?? 0);
    if (scanned >= BIG_SCAN_ROWS) {
      warnings.push(
        `Sequential scan reads ~${Math.round(scanned)} rows${n["Filter"] ? " to filter them" : ""} — an index on the filtered columns may help.`,
      );
    }
  }
  if (n["Sort Space Type"] === "Disk") warnings.push("Sort spilled to disk — consider more work_mem or an index in sort order.");
  if ((num(n["Hash Batches"]) ?? 1) > 1) warnings.push("Hash table spilled to disk in batches — consider more work_mem.");
  if (analyzed) {
    const m = misestimate(estRows, actualRows);
    if (m) warnings.push(m);
  }
  return {
    label: label(n),
    detail: detail(n),
    estRows,
    actualRows: analyzed ? actualRows : undefined,
    loops: analyzed ? loops : undefined,
    cost: num(n["Total Cost"]),
    timeMs: analyzed && total !== undefined ? total * (loops ?? 1) : undefined,
    warnings,
    children: (n.Plans ?? []).map((c) => convert(c, analyzed)),
  };
}

export function parsePostgresPlan(json: unknown, analyzed: boolean): Plan {
  const top = (Array.isArray(json) ? json[0] : json) as Record<string, unknown> | undefined;
  const root = top?.["Plan"] as PgNode | undefined;
  if (!root) throw new Error("not a PostgreSQL JSON plan");
  return {
    root: convert(root, analyzed),
    analyzed,
    totalTimeMs: num(top?.["Execution Time"]),
    planningTimeMs: num(top?.["Planning Time"]),
  };
}
