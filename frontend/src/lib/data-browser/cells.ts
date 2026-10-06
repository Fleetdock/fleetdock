import type { BrowseColumn } from "../types";
import type { Mutation } from "./pending";

// Helpers for showing and copying grid cells. Values arrive as strings (or
// null) whatever the column type; the type name decides alignment, styling
// and how a value is written out when copied as JSON or SQL.

export type CellKind = "number" | "bool" | "json" | "temporal" | "text";

const NUMBER_RE =
  /^(tinyint|smallint|mediumint|int|integer|bigint|int2|int4|int8|serial|bigserial|smallserial|serial2|serial4|serial8|numeric|decimal|dec|real|double|float|float4|float8|money|oid|bit)\b/;
const TEMPORAL_RE = /^(date|time|timestamp|timestamptz|timetz|datetime|year|interval)\b/;

export function cellKind(type: string): CellKind {
  const t = type.trim().toLowerCase();
  if (t === "boolean" || t === "bool") return "bool";
  if (t === "json" || t === "jsonb") return "json";
  if (NUMBER_RE.test(t)) return "number";
  if (TEMPORAL_RE.test(t)) return "temporal";
  return "text";
}

/** A browse cell longer than 4 KB arrives cut short with a trailing ellipsis. */
export const isTruncated = (v: string | null) => v !== null && v.length > 4096 && v.endsWith("…");

/** prettyValue formats JSON for the value panel; anything else is returned as is. */
export function prettyValue(v: string, kind: CellKind): string {
  if (kind !== "json") return v;
  try {
    return JSON.stringify(JSON.parse(v), null, 2);
  } catch {
    return v;
  }
}

/** compactCount formats a row estimate for the sidebar: 950, 12.4k, 3.1M. */
export function compactCount(n: number): string {
  if (n < 1000) return String(Math.max(0, Math.round(n)));
  return new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 }).format(n).toLowerCase();
}

// ---- copying ----

function tsvField(v: string | null): string {
  if (v === null) return "";
  return /[\t\n\r"]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

/** toTSV renders rows as tab-separated text that pastes into a spreadsheet. */
export function toTSV(columns: BrowseColumn[], rows: (string | null)[][], header = false): string {
  const lines = rows.map((r) => r.map(tsvField).join("\t"));
  if (header) lines.unshift(columns.map((c) => tsvField(c.name)).join("\t"));
  return lines.join("\n");
}

const PLAIN_NUMBER = /^-?(0|[1-9]\d*)(\.\d+)?([eE][-+]?\d+)?$/;

function jsonValue(v: string | null, kind: CellKind): unknown {
  if (v === null) return null;
  if (kind === "number" && PLAIN_NUMBER.test(v) && String(Number(v)) === v) return Number(v);
  if (kind === "bool") {
    const t = v.toLowerCase();
    if (t === "true" || t === "t" || t === "1") return true;
    if (t === "false" || t === "f" || t === "0") return false;
  }
  if (kind === "json") {
    try {
      return JSON.parse(v);
    } catch {
      return v;
    }
  }
  return v;
}

/** toJSON renders rows as a JSON array of objects keyed by column name. */
export function toJSON(columns: BrowseColumn[], rows: (string | null)[][]): string {
  const kinds = columns.map((c) => cellKind(c.type));
  const out = rows.map((r) => Object.fromEntries(columns.map((c, i) => [c.name, jsonValue(r[i], kinds[i])])));
  return JSON.stringify(out, null, 2);
}

export type Dialect = "postgres" | "mysql";

export const dialectOf = (engine: string | undefined): Dialect => (engine === "postgres" ? "postgres" : "mysql");

export function quoteIdent(name: string, d: Dialect): string {
  return d === "postgres" ? `"${name.replace(/"/g, '""')}"` : `\`${name.replace(/`/g, "``")}\``;
}

/** quoteTable quotes a table identifier as the API uses it ("schema.table" on PostgreSQL). */
export function quoteTable(table: string, d: Dialect, schema?: string): string {
  if (d === "postgres" && schema) {
    const bare = table.startsWith(`${schema}.`) ? table.slice(schema.length + 1) : table;
    return `${quoteIdent(schema, d)}.${quoteIdent(bare, d)}`;
  }
  return quoteIdent(table, d);
}

export function sqlLiteral(v: string | null, kind: CellKind, d: Dialect): string {
  if (v === null) return "NULL";
  if (kind === "number" && PLAIN_NUMBER.test(v)) return v;
  if (kind === "bool" && d === "postgres" && /^(true|false)$/i.test(v)) return v.toUpperCase();
  let s = v.replace(/'/g, "''");
  // MySQL treats backslash as an escape inside string literals by default.
  if (d === "mysql") s = s.replace(/\\/g, "\\\\");
  return `'${s}'`;
}

/** toInsertSQL renders rows as INSERT statements for the given dialect. */
export function toInsertSQL(
  quotedTable: string,
  columns: BrowseColumn[],
  rows: (string | null)[][],
  d: Dialect,
): string {
  const kinds = columns.map((c) => cellKind(c.type));
  const cols = columns.map((c) => quoteIdent(c.name, d)).join(", ");
  return rows
    .map((r) => `INSERT INTO ${quotedTable} (${cols}) VALUES (${r.map((v, i) => sqlLiteral(v, kinds[i], d)).join(", ")});`)
    .join("\n");
}

/**
 * mutationSQL shows a pending change as the SQL it amounts to, for review
 * before saving. It is only a preview: the API applies changes through its
 * typed row endpoints, never by running this text.
 */
export function mutationSQL(m: Mutation, quotedTable: string, columns: BrowseColumn[], d: Dialect): string {
  const kind = (name: string) => cellKind(columns.find((c) => c.name === name)?.type ?? "");
  const lit = (name: string, v: string | null) => sqlLiteral(v, kind(name), d);
  const where = (key: Record<string, string | null>) =>
    Object.entries(key)
      .map(([k, v]) => (v === null ? `${quoteIdent(k, d)} IS NULL` : `${quoteIdent(k, d)} = ${lit(k, v)}`))
      .join(" AND ");
  switch (m.kind) {
    case "delete":
      return `DELETE FROM ${quotedTable} WHERE ${where(m.key)};`;
    case "update":
      return `UPDATE ${quotedTable} SET ${Object.entries(m.values)
        .map(([k, v]) => `${quoteIdent(k, d)} = ${lit(k, v)}`)
        .join(", ")} WHERE ${where(m.key)};`;
    case "insert": {
      const entries = Object.entries(m.values);
      if (entries.length === 0) {
        return d === "postgres" ? `INSERT INTO ${quotedTable} DEFAULT VALUES;` : `INSERT INTO ${quotedTable} () VALUES ();`;
      }
      return `INSERT INTO ${quotedTable} (${entries.map(([k]) => quoteIdent(k, d)).join(", ")}) VALUES (${entries
        .map(([k, v]) => lit(k, v))
        .join(", ")});`;
    }
  }
}

/** estimateWidth guesses a starting column width from its header and sample values. */
export function estimateWidth(column: BrowseColumn, values: (string | null)[]): number {
  let chars = Math.max(column.name.length + 3, Math.min(column.type.length, 18));
  for (const v of values.slice(0, 60)) {
    if (v === null) continue;
    const firstLine = v.length > 80 ? 80 : (v.indexOf("\n") >= 0 ? v.indexOf("\n") : v.length);
    if (firstLine > chars) chars = firstLine;
  }
  return Math.round(Math.max(72, Math.min(360, chars * 7.4 + 28)));
}
