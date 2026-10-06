import type { BrowseColumn, RowValues } from "../types";

// Pending holds the unsaved edits of one table tab: changed cells of existing
// rows, new rows and rows marked for deletion. Nothing reaches the database
// until the user saves; then toMutations lists what to send and applyResults
// drops what succeeded and keeps the failures (with their error) on screen.

export interface PendingUpdate {
  key: RowValues;
  /** Only the changed columns, with their new values. */
  values: RowValues;
}

export interface PendingInsert {
  id: string;
  /** Columns absent from values are inserted as DEFAULT. */
  values: RowValues;
}

export interface Pending {
  updates: Record<string, PendingUpdate>;
  inserts: PendingInsert[];
  deletes: Record<string, RowValues>;
  /** Error of the last save, by change id (rowId or insert id). */
  errors: Record<string, string>;
  seq: number;
}

export const emptyPending: Pending = { updates: {}, inserts: [], deletes: {}, errors: {}, seq: 0 };

/** keyOf extracts the row-key values of a row. */
export function keyOf(columns: BrowseColumn[], key: string[], row: (string | null)[]): RowValues {
  const out: RowValues = {};
  for (const k of key) out[k] = row[columns.findIndex((c) => c.name === k)] ?? null;
  return out;
}

/** rowIdOf is a stable identity for a row: its key values, in key order. */
export function rowIdOf(key: string[], values: RowValues): string {
  return JSON.stringify(key.map((k) => values[k] ?? null));
}

export function changeCount(p: Pending): number {
  return Object.keys(p.updates).length + p.inserts.length + Object.keys(p.deletes).length;
}

export type PendingAction =
  /** Edit a cell of an existing row; editing back to `original` reverts it. */
  | { type: "set"; rowId: string; key: RowValues; column: string; value: string | null; original: string | null }
  | { type: "revertCell"; rowId: string; column: string }
  | { type: "revertRow"; rowId: string }
  | { type: "insert"; columns: BrowseColumn[] }
  | { type: "duplicate"; columns: BrowseColumn[]; row: (string | null)[]; key: string[] }
  /** Set (or with `value: undefined`, reset to DEFAULT) a cell of a new row. */
  | { type: "setNew"; id: string; column: string; value: string | null | undefined }
  | { type: "removeNew"; id: string }
  | { type: "delete"; rows: { rowId: string; key: RowValues }[] }
  | { type: "undelete"; rowId: string }
  | { type: "discard" }
  | { type: "results"; results: MutationResult[] };

function dropKey<T>(rec: Record<string, T>, k: string): Record<string, T> {
  if (!(k in rec)) return rec;
  const next = { ...rec };
  delete next[k];
  return next;
}

/** newRowValues starts a new row: DEFAULT where the column has one, else NULL or empty. */
function newRowValues(columns: BrowseColumn[]): RowValues {
  const values: RowValues = {};
  for (const c of columns) {
    if (c.has_default) continue;
    values[c.name] = c.nullable ? null : "";
  }
  return values;
}

export function pendingReducer(p: Pending, a: PendingAction): Pending {
  switch (a.type) {
    case "set": {
      const cur = p.updates[a.rowId];
      const values = { ...(cur?.values ?? {}) };
      if (a.value === a.original) delete values[a.column];
      else values[a.column] = a.value;
      const updates =
        Object.keys(values).length === 0
          ? dropKey(p.updates, a.rowId)
          : { ...p.updates, [a.rowId]: { key: cur?.key ?? a.key, values } };
      return { ...p, updates, errors: dropKey(p.errors, a.rowId) };
    }
    case "revertCell": {
      const cur = p.updates[a.rowId];
      if (!cur || !(a.column in cur.values)) return p;
      const values = { ...cur.values };
      delete values[a.column];
      const updates =
        Object.keys(values).length === 0 ? dropKey(p.updates, a.rowId) : { ...p.updates, [a.rowId]: { ...cur, values } };
      return { ...p, updates };
    }
    case "revertRow":
      return {
        ...p,
        updates: dropKey(p.updates, a.rowId),
        deletes: dropKey(p.deletes, a.rowId),
        errors: dropKey(p.errors, a.rowId),
      };
    case "insert": {
      const seq = p.seq + 1;
      return { ...p, seq, inserts: [{ id: `new-${seq}`, values: newRowValues(a.columns) }, ...p.inserts] };
    }
    case "duplicate": {
      // Copy every value except the key columns, which must differ: those
      // fall back to their default (an identity/serial) or stay to be filled.
      const seq = p.seq + 1;
      const values = newRowValues(a.columns);
      a.columns.forEach((c, i) => {
        if (!a.key.includes(c.name)) values[c.name] = a.row[i];
      });
      return { ...p, seq, inserts: [{ id: `new-${seq}`, values }, ...p.inserts] };
    }
    case "setNew":
      return {
        ...p,
        inserts: p.inserts.map((r) => {
          if (r.id !== a.id) return r;
          const values = { ...r.values };
          if (a.value === undefined) delete values[a.column];
          else values[a.column] = a.value;
          return { ...r, values };
        }),
        errors: dropKey(p.errors, a.id),
      };
    case "removeNew":
      return { ...p, inserts: p.inserts.filter((r) => r.id !== a.id), errors: dropKey(p.errors, a.id) };
    case "delete": {
      const deletes = { ...p.deletes };
      let updates = p.updates;
      for (const r of a.rows) {
        deletes[r.rowId] = r.key;
        updates = dropKey(updates, r.rowId);
      }
      return { ...p, deletes, updates };
    }
    case "undelete":
      return { ...p, deletes: dropKey(p.deletes, a.rowId), errors: dropKey(p.errors, a.rowId) };
    case "discard":
      return { ...emptyPending, seq: p.seq };
    case "results": {
      let { updates, deletes, inserts } = p;
      const errors: Record<string, string> = {};
      for (const r of a.results) {
        if (!r.ok) {
          errors[r.id] = r.error ?? "Failed";
          continue;
        }
        if (r.kind === "update") updates = dropKey(updates, r.id);
        else if (r.kind === "delete") deletes = dropKey(deletes, r.id);
        else inserts = inserts.filter((x) => x.id !== r.id);
      }
      return { ...p, updates, deletes, inserts, errors };
    }
  }
}

export type Mutation =
  | { kind: "delete"; id: string; key: RowValues }
  | { kind: "update"; id: string; key: RowValues; values: RowValues }
  | { kind: "insert"; id: string; values: RowValues };

export type MutationResult = { kind: Mutation["kind"]; id: string; ok: boolean; error?: string };

/**
 * toMutations orders the pending work deletes → updates → inserts, so that a
 * row deleted to make room for a new one with the same key goes first.
 */
export function toMutations(p: Pending): Mutation[] {
  const out: Mutation[] = [];
  for (const [id, key] of Object.entries(p.deletes)) out.push({ kind: "delete", id, key });
  for (const [id, u] of Object.entries(p.updates)) out.push({ kind: "update", id, key: u.key, values: u.values });
  // Inserts are kept newest-first for display; send them in creation order.
  for (const r of [...p.inserts].reverse()) out.push({ kind: "insert", id: r.id, values: r.values });
  return out;
}
