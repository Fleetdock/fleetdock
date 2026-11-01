import type { Completion } from "@codemirror/autocomplete";
import type { SQLNamespace } from "@codemirror/lang-sql";

import type { ColumnCatalog } from "./types";

// Builds the SQL editor's completion namespace from a database's column
// catalog: tables (with their kind) whose children are their columns (with
// their type). PostgreSQL nests tables under schemas, and "public" tables also
// complete unqualified.

export interface SqlCompletion {
  schema: SQLNamespace;
  defaultSchema?: string;
}

function tableNode(t: ColumnCatalog["tables"][number]): SQLNamespace {
  const self: Completion = { label: t.name, type: t.kind === "view" ? "class" : "type", detail: t.kind };
  const children: Completion[] = t.columns.map((c) => ({ label: c.name, type: "property", detail: c.type }));
  return { self, children };
}

export function sqlCompletion(cat: ColumnCatalog | undefined, postgres: boolean): SqlCompletion | undefined {
  if (!cat) return undefined;
  if (!postgres) {
    const schema: Record<string, SQLNamespace> = {};
    for (const t of cat.tables) schema[t.name] = tableNode(t);
    return { schema };
  }
  const schemas: Record<string, Record<string, SQLNamespace>> = {};
  for (const t of cat.tables) {
    const s = t.schema || "public";
    (schemas[s] ??= {})[t.name] = tableNode(t);
  }
  const schema: Record<string, SQLNamespace> = {};
  for (const [name, tables] of Object.entries(schemas)) {
    schema[name] = { self: { label: name, type: "namespace", detail: "schema" }, children: tables };
  }
  return { schema, defaultSchema: schemas.public ? "public" : undefined };
}
