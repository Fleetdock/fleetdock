// A query plan in one shape for every engine, so one component can draw it.

export interface PlanNode {
  /** What the step does: "Seq Scan on items i", "Unique index lookup on o". */
  label: string;
  /** Condition, key, sort key… */
  detail?: string;
  /** Rows the planner expected (per loop). */
  estRows?: number;
  /** Rows actually produced (per loop; analyzed plans only). */
  actualRows?: number;
  loops?: number;
  /** Planner cost (engine units, cumulative where the engine reports it so). */
  cost?: number;
  /** Actual time including children, in ms, across all loops (analyzed plans only). */
  timeMs?: number;
  /** Plain-language problems worth a look. */
  warnings: string[];
  children: PlanNode[];
}

export interface Plan {
  root: PlanNode;
  analyzed: boolean;
  /** Execution time of the whole statement, when the engine reports it. */
  totalTimeMs?: number;
  planningTimeMs?: number;
}

/** A scan over this many rows or more is worth an index. */
export const BIG_SCAN_ROWS = 1000;

/** Estimates off by this factor (either way) mislead the planner. */
export const MISESTIMATE_FACTOR = 10;

export function num(v: unknown): number | undefined {
  if (typeof v === "number" && Number.isFinite(v)) return v;
  if (typeof v === "string" && v.trim() !== "" && Number.isFinite(Number(v))) return Number(v);
  return undefined;
}

/** misestimate returns a warning when estimate and reality differ a lot. */
export function misestimate(est: number | undefined, actual: number | undefined): string | undefined {
  if (est === undefined || actual === undefined) return undefined;
  const hi = Math.max(est, actual);
  const lo = Math.max(1, Math.min(est, actual));
  if (hi < 100 || hi / lo < MISESTIMATE_FACTOR) return undefined;
  const factor = Math.round(hi / lo);
  return `Row estimate off by ${factor}× (expected ${Math.round(est)}, got ${Math.round(actual)}) — table statistics may be stale; run ANALYZE.`;
}

export function walk(n: PlanNode, fn: (n: PlanNode, depth: number) => void, depth = 0) {
  fn(n, depth);
  for (const c of n.children) walk(c, fn, depth + 1);
}
