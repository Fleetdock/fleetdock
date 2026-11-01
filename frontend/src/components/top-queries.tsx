"use client";

import { useState } from "react";

import {
  Badge,
  Button,
  Callout,
  CopyButton,
  EmptyState,
  Input,
  SegmentedControl,
  Table,
  TableSkeleton,
  Toolbar,
  buttonClass,
} from "@/components/ui";
import { friendlyError } from "@/lib/errors";
import { useTopQueries } from "@/lib/hooks";

type Sort = "total" | "mean" | "calls";

const SORT_OPTIONS: { value: Sort; label: string }[] = [
  { value: "total", label: "Total time" },
  { value: "mean", label: "Slowest per run" },
  { value: "calls", label: "Most frequent" },
];

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
  const { data, isLoading, error, refetch } = useTopQueries(instanceId, sort, database.trim());

  if (error) {
    return (
      <Callout
        tone="danger"
        title="Couldn't load top queries"
        actions={
          <Button size="sm" onClick={() => void refetch()}>
            Try again
          </Button>
        }
      >
        {friendlyError(error)}
      </Callout>
    );
  }
  if (data && !data.available) {
    return <Callout tone="info">{data.note ?? "This server does not collect statement statistics."}</Callout>;
  }

  const queries = data?.queries ?? [];
  const totalMs = queries.reduce((a, q) => a + q.total_ms, 0) || 1;
  return (
    <div className="card">
      {/* The toolbar stays mounted while a new sort or filter loads, so typing in the filter keeps focus. */}
      <Toolbar className="m-0 px-3 py-2.5">
        <SegmentedControl options={SORT_OPTIONS} value={sort} onChange={setSort} label="Sort by" size="sm" />
        <Input
          size="sm"
          className="max-w-56"
          placeholder="Only database…"
          value={database}
          onChange={(e) => setDatabase(e.target.value)}
          aria-label="Only statements run in this database"
        />
        <span className="toolbar-spacer" />
        {data?.stats_since ? (
          <span className="text-sm text-muted">since {new Date(data.stats_since).toLocaleString()}</span>
        ) : null}
      </Toolbar>
      {isLoading || !data ? (
        <TableSkeleton bare rows={5} columns={5} />
      ) : queries.length === 0 ? (
        <EmptyState
          plain
          title="No statements recorded yet"
          hint={
            database.trim()
              ? "Nothing has run in that database since statistics were last reset. Clear the filter to see every database."
              : "Statements appear here once applications start running queries on this server."
          }
        />
      ) : (
        <Table bare sticky wrapClassName="max-h-140 overflow-y-auto">
          <thead>
            <tr>
              <th>Statement</th>
              <th className="num">Calls</th>
              <th className="num">Total</th>
              <th className="num">Per run</th>
              <th className="num">Rows</th>
            </tr>
          </thead>
          <tbody>
            {queries.map((q, i) => (
              <tr key={i}>
                <td className="max-w-xl">
                  <div className="flex items-start gap-2">
                    <code className="flex-1 text-sm whitespace-pre-wrap break-words">
                      {q.query.length > 600 ? `${q.query.slice(0, 600)}…` : q.query}
                    </code>
                    <CopyButton value={q.query} className={buttonClass({ variant: "ghost", size: "sm" })} />
                  </div>
                  <div className="row mt-1 text-sm text-muted">
                    <span>{q.database || "—"}</span>
                    {q.no_index_used ? <Badge tone="warning">{q.no_index_used} runs without a good index</Badge> : null}
                    {q.cache_hit_pct !== undefined && q.cache_hit_pct < 90 ? (
                      <Badge tone="warning">cache hit {q.cache_hit_pct.toFixed(0)}%</Badge>
                    ) : null}
                  </div>
                </td>
                <td className="num">{q.calls.toLocaleString()}</td>
                <td className="num whitespace-nowrap">
                  {ms(q.total_ms)}
                  <div className="text-sm text-muted">{((q.total_ms / totalMs) * 100).toFixed(0)}%</div>
                </td>
                <td className="num whitespace-nowrap">{ms(q.mean_ms)}</td>
                <td className="num whitespace-nowrap">
                  {q.rows.toLocaleString()}
                  {q.rows_examined !== undefined ? (
                    <div className="text-sm text-muted">{q.rows_examined.toLocaleString()} examined</div>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
    </div>
  );
}
