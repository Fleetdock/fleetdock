import type { ImageValue, RowChange } from "../types";

// Shaping the change journal for the data browser's History panel.

/** displayValue renders a journaled value the way the grid shows cells. */
export function displayValue(v: ImageValue | undefined): string {
  if (v === undefined) return "";
  if (v === null) return "NULL";
  if (typeof v === "object") {
    const bytes = Math.floor((v.b64.length * 3) / 4) - (v.b64.endsWith("==") ? 2 : v.b64.endsWith("=") ? 1 : 0);
    return `(binary, ${bytes} byte${bytes === 1 ? "" : "s"})`;
  }
  return v;
}

/** sameValue compares two journaled values. */
export function sameValue(a: ImageValue | undefined, b: ImageValue | undefined): boolean {
  if (a === undefined || b === undefined) return a === b;
  if (a === null || b === null) return a === b;
  if (typeof a === "object" || typeof b === "object") {
    return typeof a === "object" && typeof b === "object" && a.b64 === b.b64;
  }
  return a === b;
}

export interface ColumnDiff {
  column: string;
  before?: ImageValue;
  after?: ImageValue;
}

/**
 * changedColumns lists what a change did per column: for an update the
 * columns whose value changed; for an insert or delete every column.
 */
export function changedColumns(c: RowChange): ColumnDiff[] {
  const out: ColumnDiff[] = [];
  for (const col of c.columns) {
    const before = c.before?.[col];
    const after = c.after?.[col];
    if (c.op === "update" && sameValue(before, after)) continue;
    out.push({ column: col, before: c.op === "insert" ? undefined : before, after: c.op === "delete" ? undefined : after });
  }
  return out;
}

/** keyLabel renders a row key: "id = 7" or "region = eu, num = 1". */
export function keyLabel(c: RowChange): string {
  return Object.entries(c.key ?? {})
    .map(([k, v]) => `${k} = ${displayValue(v)}`)
    .join(", ");
}

export interface ChangeGroup {
  /** The save's batch id, or the change id for a change of its own. */
  id: string;
  batchId?: string;
  changes: RowChange[];
  at: string;
  user?: string;
  source: RowChange["source"];
  /** Changes in the group that can still be undone. */
  undoable: number;
}

/**
 * groupByBatch groups consecutive changes of one save (newest first, as
 * listed) so the panel can offer "Undo this save".
 */
export function groupByBatch(changes: RowChange[]): ChangeGroup[] {
  const out: ChangeGroup[] = [];
  for (const c of changes) {
    const last = out[out.length - 1];
    if (last && c.batch_id && last.batchId === c.batch_id) {
      last.changes.push(c);
      if (c.undoable && !c.undone_at) last.undoable++;
      continue;
    }
    out.push({
      id: c.batch_id ?? c.id,
      batchId: c.batch_id,
      changes: [c],
      at: c.created_at,
      user: c.user_email,
      source: c.source,
      undoable: c.undoable && !c.undone_at ? 1 : 0,
    });
  }
  return out;
}
