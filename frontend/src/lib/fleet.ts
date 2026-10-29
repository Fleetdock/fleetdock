import type { FleetTarget } from "./types";

export interface MergedResults {
  /** Result columns, the union over databases in first-seen order. */
  columns: string[];
  rows: { database: string; server: string; cells: (string | null)[] }[];
  /** Databases whose rows were cut to fit the run's storage. */
  trimmed: string[];
}

/**
 * mergeResults lines up the first result set of every database that
 * returned rows into one table: the union of their columns, each row led by
 * its database and server. A column a database lacks is null in its rows.
 */
export function mergeResults(targets: FleetTarget[]): MergedResults {
  const columns: string[] = [];
  const index = new Map<string, number>();
  const parts: { t: FleetTarget; cols: string[]; rows: (string | null)[][] }[] = [];
  const trimmed: string[] = [];
  for (const t of targets) {
    const res = t.results?.results.find((r) => r.columns.length > 0);
    if (!res) continue;
    if (t.results?.trimmed || res.truncated) trimmed.push(t.database_name);
    for (const c of res.columns) {
      if (!index.has(c)) {
        index.set(c, columns.length);
        columns.push(c);
      }
    }
    parts.push({ t, cols: res.columns, rows: res.rows });
  }
  const rows: MergedResults["rows"] = [];
  for (const p of parts) {
    for (const r of p.rows) {
      const cells: (string | null)[] = new Array(columns.length).fill(null);
      p.cols.forEach((c, i) => {
        cells[index.get(c)!] = r[i] ?? null;
      });
      rows.push({ database: p.t.database_name, server: p.t.instance_name, cells });
    }
  }
  return { columns, rows, trimmed };
}
