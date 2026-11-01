// Query results shown in the data grid: unique column names and sorting done
// in the browser (the rows are all there).

import type { BrowseColumn, SortKey } from "../types";

/** resultColumns names result columns uniquely (SELECT a.id, b.id gives "id" and "id (2)"). */
export function resultColumns(names: string[]): BrowseColumn[] {
  const seen = new Map<string, number>();
  return names.map((n) => {
    const k = (seen.get(n) ?? 0) + 1;
    seen.set(n, k);
    return { name: k === 1 ? n : `${n} (${k})`, type: "", nullable: true, has_default: false, generated: false };
  });
}

/**
 * withNumericTypes marks columns whose every value is a number, so they
 * align right like table columns do (results carry no column types).
 */
export function withNumericTypes(columns: BrowseColumn[], rows: (string | null)[][]): BrowseColumn[] {
  return columns.map((c, i) => {
    let seen = false;
    for (const r of rows) {
      const v = r[i];
      if (v === null) continue;
      if (!/^-?\d+(\.\d+)?$/.test(v)) return c;
      seen = true;
    }
    return seen ? { ...c, type: "numeric" } : c;
  });
}

const num = (v: string) => (v.trim() !== "" && Number.isFinite(Number(v)) ? Number(v) : null);

/** compareValues orders numbers numerically, text naturally, NULL last. */
export function compareValues(a: string | null, b: string | null): number {
  if (a === b) return 0;
  if (a === null) return 1;
  if (b === null) return -1;
  const x = num(a);
  const y = num(b);
  if (x !== null && y !== null) return x - y;
  return a.localeCompare(b, undefined, { numeric: true, sensitivity: "base" });
}

/** sortRowIndexes returns row indexes in the order of the sort keys; NULLs stay last either way. */
export function sortRowIndexes(rows: (string | null)[][], columns: BrowseColumn[], sort: SortKey[]): number[] {
  const idx = rows.map((_, i) => i);
  const keys = sort
    .map((s) => ({ i: columns.findIndex((c) => c.name === s.column), desc: s.desc }))
    .filter((k) => k.i >= 0);
  if (keys.length === 0) return idx;
  return idx.sort((ra, rb) => {
    for (const k of keys) {
      const a = rows[ra][k.i];
      const b = rows[rb][k.i];
      if (a === null || b === null) {
        const c = compareValues(a, b);
        if (c !== 0) return c;
        continue;
      }
      const c = compareValues(a, b);
      if (c !== 0) return k.desc ? -c : c;
    }
    return ra - rb;
  });
}
