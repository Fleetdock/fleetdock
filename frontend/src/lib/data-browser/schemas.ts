// Which PostgreSQL schemas the data browser's table tree and the ER diagram
// show. One choice per database, shared by both and remembered in the
// browser; "all" keeps including schemas created later.

import { loadJSON, save } from "./storage";

export type SchemaSelection = { all: true } | { all: false; schemas: string[] };
export type SchemaSummary = {
  name: string;
  tables: number;
  views: number;
  owner?: string;
  comment?: string;
  bytes?: number;
};

const key = (databaseId: string) => `schemas:${databaseId}`;

const isSelection = (v: unknown): v is SchemaSelection =>
  typeof v === "object" &&
  v !== null &&
  ((v as { all: unknown }).all === true ||
    ((v as { all: unknown }).all === false &&
      Array.isArray((v as { schemas: unknown }).schemas) &&
      (v as { schemas: unknown[] }).schemas.every((s) => typeof s === "string")));

export function loadSelection(databaseId: string): SchemaSelection | null {
  return loadJSON<SchemaSelection | null>(key(databaseId), null, (v): v is SchemaSelection | null => v === null || isSelection(v));
}

export function saveSelection(databaseId: string, sel: SchemaSelection) {
  save(key(databaseId), JSON.stringify(sel.all ? sel : { all: false, schemas: sel.schemas.slice(0, 500) }));
}

/** summarizeSchemas counts tables and views per schema: public first, then by name. */
export function summarizeSchemas(items: { schema: string; kind: string }[]): SchemaSummary[] {
  const m = new Map<string, SchemaSummary>();
  for (const i of items) {
    if (!i.schema) continue;
    let s = m.get(i.schema);
    if (!s) m.set(i.schema, (s = { name: i.schema, tables: 0, views: 0 }));
    if (i.kind === "table") s.tables++;
    else if (i.kind === "view") s.views++;
  }
  return [...m.values()].sort((a, b) =>
    a.name === "public" ? -1 : b.name === "public" ? 1 : a.name.localeCompare(b.name),
  );
}

/**
 * mergeSchemas adds the schemas the server lists to a summary built from the
 * table list, so schemas without tables or views show up too. Counts from the
 * table list win; owner, comment and size come from the server.
 */
export function mergeSchemas(summary: SchemaSummary[], server: SchemaSummary[] | undefined): SchemaSummary[] {
  if (!server?.length) return summary;
  const m = new Map(summary.map((s) => [s.name, { ...s }]));
  for (const s of server) {
    const have = m.get(s.name);
    if (have) {
      have.owner = s.owner;
      have.comment = s.comment;
      have.bytes = s.bytes;
    } else {
      m.set(s.name, { ...s });
    }
  }
  return [...m.values()].sort((a, b) =>
    a.name === "public" ? -1 : b.name === "public" ? 1 : a.name.localeCompare(b.name),
  );
}

/**
 * defaultSelection is the starting point for a database seen for the first
 * time: everything when there is one schema, otherwise public (when it has
 * tables) or the schema with the most tables.
 */
export function defaultSelection(summary: SchemaSummary[]): SchemaSelection {
  if (summary.length <= 1) return { all: true };
  const pub = summary.find((s) => s.name === "public" && s.tables > 0);
  if (pub) return { all: false, schemas: ["public"] };
  const most = [...summary].sort((a, b) => b.tables - a.tables || a.name.localeCompare(b.name))[0];
  return { all: false, schemas: [most.name] };
}

/** resolveSelection drops schemas that are gone and falls back to the default when nothing is left. */
export function resolveSelection(stored: SchemaSelection | null, summary: SchemaSummary[]): SchemaSelection {
  if (!stored) return defaultSelection(summary);
  if (stored.all) return stored;
  const names = new Set(summary.map((s) => s.name));
  const kept = stored.schemas.filter((s) => names.has(s));
  return kept.length ? { all: false, schemas: kept } : defaultSelection(summary);
}

export const isSelected = (sel: SchemaSelection, schema: string) => sel.all || sel.schemas.includes(schema);

/** selectedSchemas lists the chosen schema names in summary order. */
export function selectedSchemas(sel: SchemaSelection, summary: SchemaSummary[]): string[] {
  return summary.filter((s) => isSelected(sel, s.name)).map((s) => s.name);
}

/** toggleSchema adds or removes one schema; removing the last one is refused. */
export function toggleSchema(sel: SchemaSelection, schema: string, summary: SchemaSummary[]): SchemaSelection {
  const cur = new Set(selectedSchemas(sel, summary));
  if (cur.has(schema)) {
    if (cur.size === 1) return sel;
    cur.delete(schema);
  } else {
    cur.add(schema);
  }
  return cur.size === summary.length ? { all: true } : { all: false, schemas: summary.map((s) => s.name).filter((n) => cur.has(n)) };
}

/** selectionLabel is what the picker button says. */
export function selectionLabel(sel: SchemaSelection, summary: SchemaSummary[]): string {
  const names = selectedSchemas(sel, summary);
  if (sel.all || names.length === summary.length) return "All schemas";
  return names.length === 1 ? names[0] : `${names.length} schemas`;
}
