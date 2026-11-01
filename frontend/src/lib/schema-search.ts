import type { SchemaSearchHit } from "./types";

/**
 * schemaHitHref opens a search hit in the data browser: its table (or view)
 * tab in its database. PostgreSQL tables are addressed as schema.table.
 */
export function schemaHitHref(h: SchemaSearchHit): string {
  const name = h.schema ? `${h.schema}.${h.table}` : h.table;
  const param = h.kind === "view" ? "view" : "table";
  return `/data?db=${encodeURIComponent(h.database_id)}&${param}=${encodeURIComponent(name)}`;
}
