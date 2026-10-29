"use client";

import Link from "next/link";
import { useState } from "react";

import { CopyButton } from "@/components/ui";
import { formatBytes } from "@/lib/format";
import { useIndexInsights } from "@/lib/hooks";
import type { IndexFinding, IndexInsights } from "@/lib/types";

/** findingsFor returns the findings of one table, keyed by index name. */
export function findingsFor(ins: IndexInsights | undefined, table: string): Record<string, IndexFinding & { kind: "unused" | "redundant" }> {
  const out: Record<string, IndexFinding & { kind: "unused" | "redundant" }> = {};
  if (!ins) return out;
  const matches = (f: IndexFinding) => f.index.table === table || `${f.index.schema}.${f.index.table}` === table;
  for (const f of ins.redundant) if (matches(f)) out[f.index.name] = { ...f, kind: "redundant" };
  for (const f of ins.unused) if (matches(f)) out[f.index.name] ??= { ...f, kind: "unused" };
  return out;
}

/**
 * IndexHealthCard summarizes indexes worth dropping: never used since the
 * server's statistics started, or redundant with another index. Each comes
 * with the DROP statement to review and run; nothing is dropped from here.
 * Hidden when the caller cannot read the database's data.
 */
export function IndexHealthCard({ databaseId, browserHref }: { databaseId: string; browserHref: string }) {
  const { data, error } = useIndexInsights(databaseId);
  const [now] = useState(() => Date.now());
  if (error || !data) return null;
  const findings = [
    ...data.redundant.map((f) => ({ ...f, kind: "redundant" as const })),
    ...data.unused.map((f) => ({ ...f, kind: "unused" as const })),
  ];
  const reclaim = findings.reduce((a, f) => a + (f.index.size_bytes ?? 0), 0);
  return (
    <div className="card panel aside-card">
      <h2 className="panel-title">Index health</h2>
      {findings.length === 0 ? (
        <p className="text-sm muted" style={{ margin: 0 }}>
          {data.indexes.length} index{data.indexes.length === 1 ? "" : "es"}, none redundant
          {data.usage_available ? " or unused" : ""}.
        </p>
      ) : (
        <>
          <p className="text-sm" style={{ margin: "0 0 .5rem" }}>
            {findings.length} index{findings.length === 1 ? "" : "es"} could go
            {reclaim > 0 ? `, freeing ${formatBytes(reclaim)}` : ""}. Each one slows down writes.
          </p>
          <ul className="index-findings">
            {findings.slice(0, 6).map((f) => (
              <li key={`${f.kind}:${f.index.schema ?? ""}.${f.index.table}.${f.index.name}`}>
                <div className="flex items-center gap-2">
                  <span className={`badge ${f.kind === "redundant" ? "badge-amber" : "badge-gray"}`}>{f.kind}</span>
                  <Link className="link-plain font-medium text-sm truncate" href={`${browserHref}&table=${encodeURIComponent(f.index.schema ? `${f.index.schema}.${f.index.table}` : f.index.table)}`}>
                    {f.index.table}.{f.index.name}
                  </Link>
                </div>
                <div className="text-sm muted">
                  {f.kind === "redundant" ? `covered by ${f.covered_by}` : f.reason}
                  {f.index.size_bytes ? ` · ${formatBytes(f.index.size_bytes)}` : ""}
                </div>
                <div className="flex items-center gap-1">
                  <code className="text-sm truncate" style={{ flex: 1 }}>{f.drop_sql}</code>
                  <CopyButton value={f.drop_sql} className="btn btn-ghost btn-sm" />
                </div>
              </li>
            ))}
          </ul>
          {findings.length > 6 ? <p className="text-sm muted" style={{ margin: ".4rem 0 0" }}>…and {findings.length - 6} more.</p> : null}
        </>
      )}
      {!data.usage_available && data.note ? <p className="text-sm muted" style={{ margin: ".5rem 0 0" }}>Usage unknown: {data.note}</p> : null}
      {data.usage_available && data.stats_since ? (
        <p className="text-sm muted" style={{ margin: ".5rem 0 0" }}>
          Usage counted since {new Date(data.stats_since).toLocaleDateString()}
          {now - new Date(data.stats_since).getTime() < 7 * 86400_000 ? " — less than a week, so \"unused\" may just mean \"not yet\"" : ""}.
        </p>
      ) : null}
    </div>
  );
}
