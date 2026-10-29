"use client";

import { useState } from "react";

import { Info } from "lucide-react";

import { CopyButton, ErrorText, Spinner } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useTopQueries } from "@/lib/hooks";

type Sort = "total" | "mean" | "calls";

function ms(v: number) {
  if (v >= 60_000) return `${(v / 60_000).toFixed(1)} min`;
  if (v >= 1000) return `${(v / 1000).toFixed(1)} s`;
  return `${v.toFixed(v >= 10 ? 0 : 2)} ms`;
}

/**
 * TopQueriesView lists the statements that cost the server the most time,
 * from pg_stat_statements or performance_schema. Statements are normalized
 * (literals replaced by placeholders), so one row is every run of a query
 * shape.
 */
export function TopQueriesView({ instanceId }: { instanceId: string }) {
  const [sort, setSort] = useState<Sort>("total");
  const [database, setDatabase] = useState("");
  const { data, isLoading, error } = useTopQueries(instanceId, sort, database.trim());

  if (isLoading) return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>;
  if (error) return <ErrorText message={error instanceof ApiError ? error.message : "Failed to load"} />;
  if (!data?.available) {
    return (
      <div className="callout callout-info flex gap-2">
        <Info size={16} style={{ flex: "none", marginTop: 2 }} aria-hidden />
        <span>{data?.note ?? "This server does not collect statement statistics."}</span>
      </div>
    );
  }
  const totalMs = data.queries.reduce((a, q) => a + q.total_ms, 0) || 1;
  return (
    <div className="card">
      <div className="flex items-center gap-2" style={{ padding: ".6rem .8rem", flexWrap: "wrap" }}>
        <div className="segmented" role="group" aria-label="Sort by">
          {(["total", "mean", "calls"] as Sort[]).map((s) => (
            <button key={s} aria-pressed={sort === s} onClick={() => setSort(s)} style={{ height: "2rem" }}>
              {s === "total" ? "Total time" : s === "mean" ? "Slowest per run" : "Most frequent"}
            </button>
          ))}
        </div>
        <input
          className="input"
          style={{ maxWidth: "14rem", height: "2rem" }}
          placeholder="Only database…"
          value={database}
          onChange={(e) => setDatabase(e.target.value)}
          aria-label="Only statements run in this database"
        />
        <span style={{ flex: 1 }} />
        {data.stats_since ? (
          <span className="text-sm muted">since {new Date(data.stats_since).toLocaleString()}</span>
        ) : null}
      </div>
      {data.queries.length === 0 ? (
        <p className="muted text-sm" style={{ padding: "0 .8rem .8rem" }}>No statements recorded yet.</p>
      ) : (
        <div style={{ maxHeight: 560, overflowY: "auto" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Statement</th>
                <th style={{ textAlign: "right" }}>Calls</th>
                <th style={{ textAlign: "right" }}>Total</th>
                <th style={{ textAlign: "right" }}>Per run</th>
                <th style={{ textAlign: "right" }}>Rows</th>
              </tr>
            </thead>
            <tbody>
              {data.queries.map((q, i) => (
                <tr key={i}>
                  <td style={{ maxWidth: "36rem" }}>
                    <div className="flex items-start gap-2">
                      <code className="text-sm" style={{ wordBreak: "break-word", whiteSpace: "pre-wrap", flex: 1 }}>
                        {q.query.length > 600 ? `${q.query.slice(0, 600)}…` : q.query}
                      </code>
                      <CopyButton value={q.query} className="btn btn-ghost btn-sm" />
                    </div>
                    <div className="text-sm muted" style={{ marginTop: ".2rem" }}>
                      {q.database || "—"}
                      {q.no_index_used ? <span className="badge badge-amber" style={{ marginLeft: ".5rem" }}>{q.no_index_used} runs without a good index</span> : null}
                      {q.cache_hit_pct !== undefined && q.cache_hit_pct < 90 ? (
                        <span className="badge badge-amber" style={{ marginLeft: ".5rem" }}>cache hit {q.cache_hit_pct.toFixed(0)}%</span>
                      ) : null}
                    </div>
                  </td>
                  <td className="num" style={{ textAlign: "right" }}>{q.calls.toLocaleString()}</td>
                  <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                    {ms(q.total_ms)}
                    <div className="text-sm muted">{((q.total_ms / totalMs) * 100).toFixed(0)}%</div>
                  </td>
                  <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>{ms(q.mean_ms)}</td>
                  <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                    {q.rows.toLocaleString()}
                    {q.rows_examined !== undefined ? <div className="text-sm muted">{q.rows_examined.toLocaleString()} examined</div> : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
