import type { ForeignKey, RowFilter } from "../types";

// Foreign-key navigation in the data browser: from a row to the row it
// references, and from a row to the rows that reference it. Keys may span
// several columns; every pair becomes one equality filter.

/** A row as shown in the grid, by column name (null = SQL NULL). */
export type RowRecord = Record<string, string | null>;

function pairFilters(from: string[], to: string[], row: RowRecord): RowFilter[] | null {
  if (from.length === 0 || from.length !== to.length) return null;
  const out: RowFilter[] = [];
  for (let i = 0; i < from.length; i++) {
    const v = row[from[i]];
    // A NULL component references nothing (MATCH SIMPLE).
    if (v === null || v === undefined) return null;
    out.push({ column: to[i], op: "eq", value: v });
  }
  return out;
}

/** Filters on the referenced table that select the row this row points at. */
export function followFilters(fk: ForeignKey, row: RowRecord): RowFilter[] | null {
  return pairFilters(fk.columns, fk.ref_columns, row);
}

/** Filters on the referencing table (fk.table) that select rows pointing at this row. */
export function referencingFilters(fk: ForeignKey, row: RowRecord): RowFilter[] | null {
  return pairFilters(fk.ref_columns, fk.columns, row);
}

/** Outgoing keys by each of their columns; the first key wins for a column in several. */
export function fkByColumn(fks: ForeignKey[]): Record<string, ForeignKey> {
  const out: Record<string, ForeignKey> = {};
  for (const fk of fks) {
    if (fk.columns.length === 0 || fk.columns.length !== fk.ref_columns.length) continue;
    for (const c of fk.columns) out[c] ??= fk;
  }
  return out;
}

/** "a = 1, b = 'x'" for tooltips and menu labels. */
export function describeFilters(filters: RowFilter[]): string {
  return filters.map((f) => `${f.column} = ${f.value}`).join(", ");
}
