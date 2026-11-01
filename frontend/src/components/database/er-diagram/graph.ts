// Pure helpers behind the ER diagram: turning the API's tables and foreign
// keys into a graph, picking which tables to show, and sizing table cards for
// the layout. No React and no React Flow here, so it is unit-tested directly.

import type { CatalogColumn, Diagram, DiagramTable } from "../../../lib/types";

/** Above this many tables the diagram opens on one table and its neighbours. */
export const LARGE_DIAGRAM = 150;
/** Columns listed on a card before the rest fold into "+N more". Key columns are always listed. */
export const MAX_CARD_COLUMNS = 24;

/** tableId names a table the way relations do: "schema.table" on PostgreSQL. */
export function tableId(t: Pick<DiagramTable, "schema" | "name">): string {
  return t.schema ? `${t.schema}.${t.name}` : t.name;
}

export interface Relation {
  id: string;
  name: string;
  source: string;
  target: string;
  columns: string[];
  refColumns: string[];
  onDelete: string;
  onUpdate: string;
  /** Every referencing column accepts NULL: the relation is optional (0..1). */
  optional: boolean;
  /** The table references itself. */
  self: boolean;
}

export interface Graph {
  tables: Map<string, DiagramTable>;
  relations: Relation[];
  /** Columns of each table that take part in a foreign key (either side). */
  keyColumns: Map<string, Set<string>>;
  /** Number of relations touching each table. */
  degree: Map<string, number>;
}

/**
 * buildGraph indexes the diagram. Relations whose ends are not listed (a table
 * the login cannot read, or one cut by truncation) are dropped.
 */
export function buildGraph(d: Diagram): Graph {
  const tables = new Map<string, DiagramTable>();
  for (const t of d.tables) tables.set(tableId(t), t);
  const keyColumns = new Map<string, Set<string>>();
  const degree = new Map<string, number>();
  const mark = (table: string, cols: string[]) => {
    let set = keyColumns.get(table);
    if (!set) keyColumns.set(table, (set = new Set()));
    for (const c of cols) set.add(c);
  };
  const relations: Relation[] = [];
  for (const fk of d.relations) {
    const source = fk.table ?? "";
    const src = tables.get(source);
    if (!src || !tables.has(fk.ref_table)) continue;
    const nullable = new Map(src.columns.map((c) => [c.name, c.nullable ?? false]));
    relations.push({
      id: `${source}\u0000${fk.name}`,
      name: fk.name,
      source,
      target: fk.ref_table,
      columns: fk.columns,
      refColumns: fk.ref_columns,
      onDelete: fk.on_delete,
      onUpdate: fk.on_update,
      optional: fk.columns.length > 0 && fk.columns.every((c) => nullable.get(c) === true),
      self: source === fk.ref_table,
    });
    mark(source, fk.columns);
    mark(fk.ref_table, fk.ref_columns);
    degree.set(source, (degree.get(source) ?? 0) + 1);
    if (source !== fk.ref_table) degree.set(fk.ref_table, (degree.get(fk.ref_table) ?? 0) + 1);
  }
  return { tables, relations, keyColumns, degree };
}

/** neighbourhood returns the tables within depth relations of start (both directions). */
export function neighbourhood(g: Graph, start: Iterable<string>, depth: number): Set<string> {
  const adj = new Map<string, string[]>();
  for (const r of g.relations) {
    if (r.self) continue;
    (adj.get(r.source) ?? adj.set(r.source, []).get(r.source)!).push(r.target);
    (adj.get(r.target) ?? adj.set(r.target, []).get(r.target)!).push(r.source);
  }
  const seen = new Set<string>();
  let frontier: string[] = [];
  for (const s of start) {
    if (g.tables.has(s) && !seen.has(s)) {
      seen.add(s);
      frontier.push(s);
    }
  }
  for (let i = 0; i < depth && frontier.length > 0; i++) {
    const next: string[] = [];
    for (const t of frontier) {
      for (const n of adj.get(t) ?? []) {
        if (!seen.has(n)) {
          seen.add(n);
          next.push(n);
        }
      }
    }
    frontier = next;
  }
  return seen;
}

/** mostConnected is the table with the most relations (ties: by name); a good place to start a large diagram. */
export function mostConnected(g: Graph): string | null {
  let best: string | null = null;
  let bestN = -1;
  for (const id of [...g.tables.keys()].sort()) {
    const n = g.degree.get(id) ?? 0;
    if (n > bestN) {
      best = id;
      bestN = n;
    }
  }
  return best;
}

/** schemasOf lists the PostgreSQL schemas present, sorted; empty on MySQL/MariaDB. */
export function schemasOf(g: Graph): string[] {
  const s = new Set<string>();
  for (const t of g.tables.values()) if (t.schema) s.add(t.schema);
  return [...s].sort();
}

export interface CardColumns {
  shown: CatalogColumn[];
  hidden: number;
}

/**
 * cardColumns picks the columns a table card lists: the first MAX_CARD_COLUMNS
 * in order, plus every key column after them, so relation lines always land
 * on a row.
 */
export function cardColumns(t: DiagramTable, keys: Set<string> | undefined): CardColumns {
  const shown = t.columns.filter((c, i) => i < MAX_CARD_COLUMNS || c.primary_key || keys?.has(c.name));
  return { shown, hidden: t.columns.length - shown.length };
}

/** Card geometry, shared by the layout (which needs sizes up front) and the CSS. */
export const CARD = { header: 40, row: 24, footer: 22, minWidth: 200, maxWidth: 340, charWidth: 7.2, padding: 56 };

/** cardSize estimates a card's rendered size from its text. */
export function cardSize(t: DiagramTable, cols: CardColumns): { width: number; height: number } {
  const longest = Math.max(
    tableId(t).length + 6,
    ...cols.shown.map((c) => c.name.length + c.type.length + 2),
  );
  const width = Math.round(Math.min(CARD.maxWidth, Math.max(CARD.minWidth, longest * CARD.charWidth + CARD.padding)));
  const height = CARD.header + Math.max(1, cols.shown.length) * CARD.row + (cols.hidden > 0 ? CARD.footer : 0) + 8;
  return { width, height };
}

/** relationLabel describes a foreign key for tooltips and screen readers. */
export function relationLabel(r: Relation): string {
  const cols = r.columns.join(", ");
  const ref = r.refColumns.join(", ");
  const rules = [
    r.onDelete && r.onDelete !== "NO ACTION" ? `on delete ${r.onDelete.toLowerCase()}` : "",
    r.onUpdate && r.onUpdate !== "NO ACTION" ? `on update ${r.onUpdate.toLowerCase()}` : "",
  ].filter(Boolean);
  return `${r.source}(${cols}) → ${r.target}(${ref})${r.optional ? ", optional" : ""}${rules.length ? `, ${rules.join(", ")}` : ""}`;
}

/** Saved node positions of one database's full diagram. */
export type SavedPositions = Record<string, { x: number; y: number }>;

export function isSavedPositions(v: unknown): v is SavedPositions {
  if (typeof v !== "object" || v === null || Array.isArray(v)) return false;
  return Object.values(v).every(
    (p) =>
      typeof p === "object" &&
      p !== null &&
      typeof (p as { x: unknown }).x === "number" &&
      typeof (p as { y: unknown }).y === "number",
  );
}

/** usableSaved reports whether saved positions cover every table to draw. */
export function usableSaved(saved: SavedPositions, ids: Iterable<string>): boolean {
  let any = false;
  for (const id of ids) {
    if (!saved[id]) return false;
    any = true;
  }
  return any;
}
