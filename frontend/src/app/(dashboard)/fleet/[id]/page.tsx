"use client";

import { Download, Pause, Play, RotateCcw, Square } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useMemo, useState } from "react";

import { SqlEditor } from "@/components/sql-editor";
import { EmptyState, ErrorText, PageHeader, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { mergeResults } from "@/lib/fleet";
import { formatDuration } from "@/lib/format";
import { downloadFleetCSV, useFleetRun, useFleetRunActions, useFleetTargets } from "@/lib/hooks";
import type { FleetRun, FleetTarget } from "@/lib/types";

const ACTIVE = new Set(["running", "canceling"]);

export default function FleetRunPage() {
  const id = String(useParams().id);
  const { data: run, isLoading } = useFleetRun(id);
  const active = run ? ACTIVE.has(run.status) : false;
  const { data: targets } = useFleetTargets(id, active);
  const [picked, setTab] = useState<"results" | "databases" | null>(null);

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 muted text-sm">
        <Spinner /> Loading…
      </div>
    );
  }
  if (!run) return <EmptyState title="Fleet run not found" />;
  // A script that writes is judged by what it changed on each database.
  const tab = picked ?? (run.writes ? "databases" : "results");

  return (
    <div>
      <PageHeader
        breadcrumbs={[{ label: "Fleet query", href: "/fleet" }, { label: `Run ${run.id.slice(0, 8)}` }]}
        title={`Fleet run on ${run.total} database${run.total === 1 ? "" : "s"}`}
        badges={<StatusBadge status={run.status} />}
        actions={<RunActions run={run} />}
      />
      <RunSummary run={run} />
      <div className="tabs" role="tablist" aria-label="Run sections">
        {(["results", "databases"] as const).map((t) => (
          <button key={t} type="button" role="tab" aria-selected={tab === t} className={`tab${tab === t ? " active" : ""}`} onClick={() => setTab(t)}>
            {t === "results" ? "Results" : "Databases"}
          </button>
        ))}
      </div>
      {!targets ? <Spinner /> : tab === "results" ? <Merged run={run} targets={targets} /> : <Targets targets={targets} />}
    </div>
  );
}

function RunActions({ run }: { run: FleetRun }) {
  const { resume, cancel, retry } = useFleetRunActions(run.id);
  const [error, setError] = useState<string | null>(null);
  const onError = (e: unknown) => setError(e instanceof ApiError ? e.message : "Something went wrong");
  return (
    <>
      {error ? <span className="text-sm" style={{ color: "var(--danger)" }}>{error}</span> : null}
      {run.status === "paused" ? (
        <button className="btn btn-primary" disabled={resume.isPending} onClick={() => resume.mutate(undefined, { onError })}>
          <Play size={15} /> Continue on the other {run.total - 1}
        </button>
      ) : null}
      {run.status === "running" || run.status === "paused" ? (
        <button className="btn" disabled={cancel.isPending} onClick={() => cancel.mutate(undefined, { onError })}>
          <Square size={14} /> Cancel
        </button>
      ) : null}
      {(run.status === "failed" || run.status === "canceled") && run.succeeded < run.total ? (
        <button className="btn" disabled={retry.isPending} onClick={() => retry.mutate(undefined, { onError })}>
          <RotateCcw size={14} /> Retry the {run.total - run.succeeded} not done
        </button>
      ) : null}
      <button className="btn" onClick={() => void downloadFleetCSV(run.id).catch(onError)}>
        <Download size={14} /> CSV
      </button>
    </>
  );
}

function RunSummary({ run }: { run: FleetRun }) {
  const done = run.succeeded + run.failed + run.skipped;
  const pct = run.total ? Math.round((100 * done) / run.total) : 0;
  return (
    <div className="card panel fleet-summary">
      {run.status === "paused" ? (
        <div className="callout callout-info flex items-center gap-2">
          <Pause size={14} aria-hidden /> The script ran on the first database. Check its result below, then continue
          on the rest or cancel.
        </div>
      ) : null}
      <div className="fleet-progress" aria-label={`${pct}% done`}>
        <span className="is-ok" style={{ width: `${(100 * run.succeeded) / Math.max(run.total, 1)}%` }} />
        <span className="is-failed" style={{ width: `${(100 * run.failed) / Math.max(run.total, 1)}%` }} />
        <span className="is-skipped" style={{ width: `${(100 * run.skipped) / Math.max(run.total, 1)}%` }} />
      </div>
      <div className="fleet-counts text-sm">
        <span>
          <strong>{run.succeeded}</strong> succeeded
        </span>
        <span>
          <strong>{run.failed}</strong> failed
        </span>
        <span>
          <strong>{run.skipped}</strong> skipped
        </span>
        <span>
          <strong>{run.running}</strong> running
        </span>
        <span className="muted">
          {run.writes ? "writes" : "read-only"} · {run.concurrency} at once · {run.timeout_seconds}s timeout
          {run.stop_on_error ? " · stops on failure" : ""}
        </span>
      </div>
      <details>
        <summary className="text-sm muted">Script</summary>
        <SqlEditor value={run.sql} onChange={() => {}} readOnly minHeight="4rem" />
      </details>
    </div>
  );
}

function Merged({ run, targets }: { run: FleetRun; targets: FleetTarget[] }) {
  const m = useMemo(() => mergeResults(targets), [targets]);
  const affected = targets.reduce((a, t) => a + (t.rows_affected ?? 0), 0);
  if (m.columns.length === 0) {
    return (
      <p className="text-sm muted">
        {run.writes
          ? `${affected.toLocaleString()} row${affected === 1 ? "" : "s"} changed ${run.finished_at ? "in all" : "so far"}. See Databases for each one.`
          : ACTIVE.has(run.status)
            ? "Waiting for results…"
            : "No rows were returned."}
      </p>
    );
  }
  return (
    <div>
      {m.trimmed.length > 0 ? (
        <p className="text-sm muted">
          Some databases returned more rows than are kept ({m.trimmed.slice(0, 5).join(", ")}
          {m.trimmed.length > 5 ? "…" : ""}); raise the row limit or narrow the query.
        </p>
      ) : null}
      <div className="card fleet-results">
        <table className="table">
          <thead>
            <tr>
              <th>database</th>
              <th>server</th>
              {m.columns.map((c) => (
                <th key={c}>{c}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {m.rows.map((r, i) => (
              <tr key={i}>
                <td className="font-medium">{r.database}</td>
                <td className="muted">{r.server}</td>
                {r.cells.map((c, j) => (
                  <td key={j} className={c === null ? "muted" : undefined}>
                    {c === null ? "NULL" : c}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function Targets({ targets }: { targets: FleetTarget[] }) {
  return (
    <div className="card">
      <table className="table">
        <thead>
          <tr>
            <th>Database</th>
            <th>Status</th>
            <th>Rows</th>
            <th>Time</th>
            <th>Message</th>
          </tr>
        </thead>
        <tbody>
          {targets.map((t) => {
            const sets = t.results?.results.filter((r) => r.columns.length > 0) ?? [];
            const rows = sets.reduce((a, r) => a + r.row_count, 0);
            const ms = t.started_at && t.finished_at ? Date.parse(t.finished_at) - Date.parse(t.started_at) : null;
            return (
              <tr key={t.position}>
                <td>
                  {t.database_id ? (
                    <Link className="link" href={`/databases/${t.database_id}`}>
                      {t.database_name}
                    </Link>
                  ) : (
                    t.database_name
                  )}
                  <div className="text-sm muted">{t.instance_name}</div>
                </td>
                <td>
                  <StatusBadge status={t.status} />
                </td>
                <td className="text-sm">
                  {!t.results
                    ? "—"
                    : sets.length > 0
                      ? `${rows.toLocaleString()} returned`
                      : `${(t.rows_affected ?? 0).toLocaleString()} changed`}
                </td>
                <td className="text-sm muted">{ms !== null ? formatDuration(ms) : "—"}</td>
                <td className="text-sm" style={{ color: t.error ? "var(--danger)" : undefined, maxWidth: "28rem" }}>
                  {t.error ?? ""}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
