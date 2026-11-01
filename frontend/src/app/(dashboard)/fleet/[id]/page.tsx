"use client";

import { Download, Play, RotateCcw, Square } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useMemo, useState } from "react";

import { SqlEditor } from "@/components/sql-editor";
import {
  Button,
  ButtonLink,
  Callout,
  DetailSkeleton,
  EmptyState,
  PageHeader,
  QueryError,
  StatusBadge,
  Table,
  TableSkeleton,
  Tabs,
} from "@/components/ui";
import { ApiError } from "@/lib/api";
import { mergeResults } from "@/lib/fleet";
import { formatDuration } from "@/lib/format";
import { downloadFleetCSV, useFleetRun, useFleetRunActions, useFleetTargets } from "@/lib/hooks";
import type { FleetRun, FleetTarget } from "@/lib/types";

const ACTIVE = new Set(["running", "canceling"]);

type RunTab = "results" | "databases";

export default function FleetRunPage() {
  const id = String(useParams().id);
  const { data: run, isLoading, error, refetch } = useFleetRun(id);
  const active = run ? ACTIVE.has(run.status) : false;
  const targetsQuery = useFleetTargets(id, active);
  const targets = targetsQuery.data;
  const [picked, setTab] = useState<RunTab | null>(null);

  if (isLoading) return <DetailSkeleton label="Loading fleet run" />;
  if (!run) {
    return (
      <div>
        <PageHeader breadcrumbs={[{ label: "Fleet query", href: "/fleet" }, { label: "Run" }]} title="Fleet run" />
        <QueryError error={error} what="fleet run" onRetry={() => void refetch()} backHref="/fleet" backLabel="Back to Fleet query" />
      </div>
    );
  }
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
      <Tabs<RunTab>
        tabs={[
          { id: "results", label: "Results" },
          { id: "databases", label: "Databases", count: run.total },
        ]}
        value={tab}
        onChange={setTab}
        label="Run sections"
        panelId="fleet-run-panel"
      />
      <div id="fleet-run-panel" role="tabpanel" aria-label={tab === "results" ? "Results" : "Databases"} tabIndex={0} className="mt-4">
        {!targets ? (
          targetsQuery.isError ? (
            <QueryError error={targetsQuery.error} what="run results" onRetry={() => void targetsQuery.refetch()} />
          ) : (
            <TableSkeleton rows={4} columns={5} />
          )
        ) : tab === "results" ? (
          <Merged run={run} targets={targets} />
        ) : (
          <Targets targets={targets} />
        )}
      </div>
    </div>
  );
}

function RunActions({ run }: { run: FleetRun }) {
  const { resume, cancel, retry } = useFleetRunActions(run.id);
  const [error, setError] = useState<string | null>(null);
  const onError = (e: unknown) => setError(e instanceof ApiError ? e.message : "Something went wrong");
  return (
    <>
      {error ? (
        <span className="text-sm text-danger" role="alert">
          {error}
        </span>
      ) : null}
      {run.status === "paused" ? (
        <Button variant="primary" icon={<Play size={15} aria-hidden />} loading={resume.isPending} onClick={() => resume.mutate(undefined, { onError })}>
          Continue on the other {run.total - 1}
        </Button>
      ) : null}
      {run.status === "running" || run.status === "paused" ? (
        <Button icon={<Square size={14} aria-hidden />} loading={cancel.isPending} onClick={() => cancel.mutate(undefined, { onError })}>
          Cancel
        </Button>
      ) : null}
      {(run.status === "failed" || run.status === "canceled") && run.succeeded < run.total ? (
        <Button icon={<RotateCcw size={14} aria-hidden />} loading={retry.isPending} onClick={() => retry.mutate(undefined, { onError })}>
          Retry the {run.total - run.succeeded} not done
        </Button>
      ) : null}
      <Button icon={<Download size={14} aria-hidden />} onClick={() => void downloadFleetCSV(run.id).catch(onError)}>
        Download CSV
      </Button>
    </>
  );
}

function RunSummary({ run }: { run: FleetRun }) {
  const done = run.succeeded + run.failed + run.skipped;
  const pct = run.total ? Math.round((100 * done) / run.total) : 0;
  return (
    <div className="card panel fleet-summary">
      {run.status === "paused" ? (
        <Callout tone="info" title="Paused after the first database">
          The script ran on the first database. Check its result below, then continue on the rest or cancel.
        </Callout>
      ) : null}
      <div className="fleet-progress" role="img" aria-label={`${pct}% done`}>
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
      <EmptyState
        title={run.writes ? "No result sets" : ACTIVE.has(run.status) ? "Waiting for results" : "No rows returned"}
        hint={run.writes
          ? `${affected.toLocaleString()} row${affected === 1 ? "" : "s"} changed ${run.finished_at ? "in all" : "so far"}. See Databases for each one.`
          : ACTIVE.has(run.status)
            ? "Waiting for results…"
            : "No rows were returned."}
        action={
          run.writes ? null : (
            <ButtonLink href="/fleet">New fleet query</ButtonLink>
          )
        }
      />
    );
  }
  return (
    <div>
      {m.trimmed.length > 0 ? (
        <Callout tone="info" className="mb-3">
          Some databases returned more rows than are kept ({m.trimmed.slice(0, 5).join(", ")}
          {m.trimmed.length > 5 ? "…" : ""}); raise the row limit or narrow the query.
        </Callout>
      ) : null}
      <Table wrapClassName="fleet-results">
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
      </Table>
    </div>
  );
}

function Targets({ targets }: { targets: FleetTarget[] }) {
  return (
    <Table>
        <thead>
          <tr>
            <th>Database</th>
            <th>Status</th>
            <th className="num">Rows</th>
            <th className="num">Time</th>
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
                <td className="num text-sm">
                  {!t.results
                    ? "—"
                    : sets.length > 0
                      ? `${rows.toLocaleString()} returned`
                      : `${(t.rows_affected ?? 0).toLocaleString()} changed`}
                </td>
                <td className="num text-sm muted">{ms !== null ? formatDuration(ms) : "—"}</td>
                <td className={`text-sm max-w-md${t.error ? " text-danger" : ""}`}>
                  {t.error ?? ""}
                </td>
              </tr>
            );
          })}
        </tbody>
    </Table>
  );
}
