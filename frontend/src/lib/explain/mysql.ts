import { BIG_SCAN_ROWS, misestimate, num, type Plan, type PlanNode } from "./types";

// MySQL and MariaDB EXPLAIN FORMAT=JSON (and MariaDB's ANALYZE FORMAT=JSON,
// which adds r_* fields with actual figures). Both nest "operations"
// (ordering, grouping, filesort, temporary tables, unions…) around a
// query_block whose "nested_loop" lists the tables in join order.

type Obj = Record<string, unknown>;

const ACCESS: Record<string, string> = {
  ALL: "Full table scan",
  index: "Full index scan",
  range: "Index range scan",
  ref: "Index lookup",
  ref_or_null: "Index lookup",
  eq_ref: "Unique index lookup",
  const: "Constant lookup",
  system: "Constant lookup",
  fulltext: "Full-text search",
  index_merge: "Index merge",
  hash: "Hash join",
};

const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

function tableNode(t: Obj, analyzed: boolean): PlanNode {
  const access = String(t.access_type ?? "");
  const name = String(t.table_name ?? "?");
  const estRows = num(t.rows_examined_per_scan) ?? num(t.rows);
  const actualRows = num(t.r_rows);
  const loops = num(t.r_loops);
  const details: string[] = [];
  if (t.key) details.push(`key ${t.key}${Array.isArray(t.used_key_parts) ? ` (${t.used_key_parts.join(", ")})` : ""}`);
  else if (Array.isArray(t.possible_keys) && t.possible_keys.length) details.push(`possible keys ${t.possible_keys.join(", ")} not used`);
  if (t.attached_condition) details.push(`where ${t.attached_condition}`);
  const warnings: string[] = [];
  const scanned = analyzed && actualRows !== undefined ? actualRows : estRows;
  if (access === "ALL" && (scanned ?? 0) >= BIG_SCAN_ROWS) {
    warnings.push(`Full scan of ${name} reads ~${Math.round(scanned!)} rows — an index on the filtered or joined columns may help.`);
  }
  if (t.using_join_buffer) warnings.push(`Joins ${name} without an index (join buffer: ${t.using_join_buffer}).`);
  if (analyzed) {
    const m = misestimate(estRows, actualRows);
    if (m) warnings.push(m);
  }
  const costInfo = isObj(t.cost_info) ? t.cost_info : undefined;
  return {
    label: `${ACCESS[access] ?? (access || "Read")} on ${name}`,
    detail: details.join(" · ") || undefined,
    estRows,
    actualRows: analyzed ? actualRows : undefined,
    loops: analyzed ? loops : num(t.loops),
    cost: num(costInfo?.prefix_cost) ?? num(t.cost),
    timeMs: analyzed ? num(t.r_total_time_ms) ?? sum(num(t.r_table_time_ms), num(t.r_other_time_ms)) : undefined,
    warnings,
    children: subqueries(t, analyzed),
  };
}

function sum(a?: number, b?: number) {
  return a === undefined && b === undefined ? undefined : (a ?? 0) + (b ?? 0);
}

function subqueries(o: Obj, analyzed: boolean): PlanNode[] {
  const out: PlanNode[] = [];
  const m = o.materialized_from_subquery;
  if (isObj(m) && isObj(m.query_block)) out.push(block(m.query_block, analyzed, "Materialized subquery"));
  for (const key of ["attached_subqueries", "subqueries"]) {
    const subs = o[key];
    if (Array.isArray(subs)) for (const s of subs) if (isObj(s) && isObj(s.query_block)) out.push(block(s.query_block, analyzed, "Subquery"));
  }
  return out;
}

// operations converts the members of an object that describe plan steps.
function operations(o: Obj, analyzed: boolean): PlanNode[] {
  const out: PlanNode[] = [];
  if (isObj(o.table)) out.push(tableNode(o.table, analyzed));
  if (Array.isArray(o.nested_loop)) {
    const kids = o.nested_loop.flatMap((x) => (isObj(x) ? operations(x, analyzed) : []));
    out.push(kids.length === 1 ? kids[0] : { label: "Nested loop join", warnings: [], children: kids });
  }
  const wrap = (key: string, lbl: (v: Obj) => string, warn: (v: Obj) => string[]) => {
    const v = o[key];
    if (!isObj(v)) return;
    const timeMs = analyzed ? num(v.r_total_time_ms) : undefined;
    out.push({ label: lbl(v), warnings: warn(v), timeMs, cost: num(isObj(v.cost_info) ? v.cost_info.sort_cost : undefined), children: operations(v, analyzed) });
  };
  const sortWarn = (v: Obj) => {
    const w: string[] = [];
    if (v.using_filesort) w.push("Sorts with a filesort — an index in ORDER BY order would avoid it.");
    if (v.using_temporary_table) w.push("Builds a temporary table.");
    return w;
  };
  wrap("ordering_operation", () => "Sort (ORDER BY)", sortWarn);
  wrap("grouping_operation", () => "Group (GROUP BY)", sortWarn);
  wrap("duplicates_removal", () => "Remove duplicates (DISTINCT)", sortWarn);
  wrap("windowing", () => "Window functions", sortWarn);
  wrap("filesort", (v) => `Sort${v.sort_key ? ` by ${v.sort_key}` : ""} (filesort)`, () => ["Sorts with a filesort — an index in ORDER BY order would avoid it."]);
  wrap("temporary_table", () => "Temporary table", () => ["Builds a temporary table."]);
  // MariaDB wrappers that only carry their content.
  for (const key of ["read_sorted_file", "buffer_result"]) {
    if (isObj(o[key])) out.push(...operations(o[key] as Obj, analyzed));
  }
  if (isObj(o.union_result)) {
    const u = o.union_result;
    const specs = Array.isArray(u.query_specifications) ? u.query_specifications : [];
    out.push({
      label: "Union",
      warnings: [],
      children: specs.flatMap((s) => (isObj(s) && isObj(s.query_block) ? [block(s.query_block, analyzed, "Part")] : [])),
    });
  }
  out.push(...subqueries(o, analyzed));
  return out;
}

function block(qb: Obj, analyzed: boolean, title = "Query"): PlanNode {
  const children = operations(qb, analyzed);
  const costInfo = isObj(qb.cost_info) ? qb.cost_info : undefined;
  const cost = num(costInfo?.query_cost) ?? num(qb.cost);
  const timeMs = analyzed ? num(qb.r_total_time_ms) : undefined;
  if (title === "Query" && children.length === 1 && cost === undefined && timeMs === undefined) return children[0];
  return {
    label: qb.message ? `${title}: ${qb.message}` : qb.select_id !== undefined && title !== "Query" ? `${title} #${qb.select_id}` : title,
    cost,
    timeMs,
    warnings: [],
    children,
  };
}

export function parseMySQLPlan(json: unknown, analyzed: boolean): Plan {
  if (!isObj(json) || !isObj(json.query_block)) throw new Error("not a MySQL/MariaDB JSON plan");
  const root = block(json.query_block, analyzed);
  const opt = isObj(json.query_optimization) ? json.query_optimization : undefined;
  return { root, analyzed, totalTimeMs: root.timeMs, planningTimeMs: num(opt?.r_total_time_ms) };
}

// MySQL's EXPLAIN ANALYZE prints a tree, one step per line, four spaces per
// level: "-> Table scan on i  (cost=0.45 rows=2) (actual time=0.0194..0.0224 rows=2 loops=1)".
const LINE = /^(\s*)-> (.*?)(?:\s+\(cost=([\d.e+]+)(?: rows=([\d.e+]+))?\))?(?:\s+\(actual time=([\d.e+]+)\.\.([\d.e+]+) rows=([\d.e+]+) loops=(\d+)\))?\s*$/;

export function parseMySQLTree(text: string): Plan {
  const roots: PlanNode[] = [];
  const stack: { depth: number; node: PlanNode }[] = [];
  for (const raw of text.split("\n")) {
    if (!raw.trim()) continue;
    const m = LINE.exec(raw);
    if (!m) {
      // A continuation line (long conditions wrap): append to the last step.
      const last = stack[stack.length - 1];
      if (last) last.node.label += ` ${raw.trim()}`;
      continue;
    }
    const depth = m[1].length;
    const estRows = num(m[4]);
    const actualRows = num(m[7]);
    const loops = num(m[8]);
    const end = num(m[6]);
    const node: PlanNode = {
      label: m[2],
      cost: num(m[3]),
      estRows,
      actualRows,
      loops,
      timeMs: end !== undefined ? end * (loops ?? 1) : undefined,
      warnings: [],
      children: [],
    };
    if (/^Table scan on /.test(node.label) && ((actualRows ?? estRows ?? 0) * (loops ?? 1)) >= BIG_SCAN_ROWS) {
      node.warnings.push(`${node.label} reads ~${Math.round((actualRows ?? estRows ?? 0) * (loops ?? 1))} rows — an index may help.`);
    }
    if (/using temporary/i.test(node.label)) node.warnings.push("Builds a temporary table.");
    const mis = misestimate(estRows, actualRows);
    if (mis) node.warnings.push(mis);
    while (stack.length && stack[stack.length - 1].depth >= depth) stack.pop();
    if (stack.length) stack[stack.length - 1].node.children.push(node);
    else roots.push(node);
    stack.push({ depth, node });
  }
  if (roots.length === 0) throw new Error("empty EXPLAIN ANALYZE output");
  const root = roots.length === 1 ? roots[0] : { label: "Query", warnings: [], children: roots };
  return { root, analyzed: true, totalTimeMs: root.timeMs };
}
