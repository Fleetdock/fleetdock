"use client";

import { Play } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { useConfirm } from "@/components/confirm";
import { engineMix, TargetPicker } from "@/components/fleet/target-picker";
import { SqlEditor } from "@/components/sql-editor";
import { EmptyState, ErrorText, PageHeader, Spinner, StatusBadge, Time } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useAllDatabases, useCreateFleetRun, useFleetRuns } from "@/lib/hooks";
import { splitStatements, writeStatements } from "@/lib/sql";

/**
 * Fleet query: write one script, pick many databases, run it on all of them
 * in the background, and read the results side by side.
 */
export default function FleetPage() {
  const router = useRouter();
  const confirm = useConfirm();
  const { data: dbs } = useAllDatabases();
  const runs = useFleetRuns();
  const create = useCreateFleetRun();
  const [sql, setSql] = useState("SELECT count(*) AS n FROM ");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [concurrency, setConcurrency] = useState(4);
  const [timeoutS, setTimeoutS] = useState(60);
  const [rowLimit, setRowLimit] = useState(100);
  const [stopOnError, setStopOnError] = useState<boolean | null>(null);
  const [canary, setCanary] = useState<boolean | null>(null);
  const [error, setError] = useState<string | null>(null);

  const writes = writeStatements(sql).length > 0;
  const n = selected.size;
  const engines = engineMix(dbs, selected);
  const stop = stopOnError ?? writes;
  const firstAlone = (canary ?? writes) && n > 1;

  const run = async () => {
    setError(null);
    if (splitStatements(sql).length === 0) return setError("Write a script first.");
    if (n === 0) return setError("Pick at least one database.");
    if (writes) {
      const ok = await confirm({
        title: `Run a script that writes on ${n} database${n === 1 ? "" : "s"}?`,
        message: (
          <>
            {firstAlone ? "It runs on the first database alone, then waits for you to look before the rest. " : ""}
            {stop ? "It stops at the first database that fails. " : "A failure on one database does not stop the others. "}
            Nothing is rolled back for you. Type <strong>{`run on ${n}`}</strong> to confirm.
          </>
        ),
        confirmLabel: "Run",
        confirmText: `run on ${n}`,
        danger: true,
      });
      if (!ok) return;
    }
    create.mutate(
      {
        sql,
        database_ids: [...selected],
        concurrency,
        timeout_seconds: timeoutS,
        row_limit: rowLimit,
        stop_on_error: stop,
        canary: firstAlone,
      },
      {
        onSuccess: (r) => router.push(`/fleet/${r.id}`),
        onError: (e) => setError(e instanceof ApiError ? e.message : "Could not start the run"),
      },
    );
  };

  return (
    <div>
      <PageHeader
        title="Fleet query"
        description="Run one SQL script on many databases at once and compare the results."
      />
      <div className="fleet-compose">
        <section className="card panel">
          <h2 className="panel-title">Script</h2>
          <SqlEditor value={sql} onChange={setSql} onRun={() => void run()} minHeight="12rem" engine={engines.length === 1 ? engines[0] : undefined} />
          <div className="fleet-options">
            <label>
              At once
              <input className="input" type="number" min={1} max={16} value={concurrency} onChange={(e) => setConcurrency(Number(e.target.value) || 1)} />
            </label>
            <label>
              Timeout (s)
              <input className="input" type="number" min={1} max={600} value={timeoutS} onChange={(e) => setTimeoutS(Number(e.target.value) || 60)} />
            </label>
            <label>
              Rows per database
              <input className="input" type="number" min={1} max={1000} value={rowLimit} onChange={(e) => setRowLimit(Number(e.target.value) || 100)} />
            </label>
            <label className="fleet-check">
              <input type="checkbox" checked={stop} onChange={(e) => setStopOnError(e.target.checked)} /> Stop at the first failure
            </label>
            <label className="fleet-check">
              <input type="checkbox" checked={canary ?? writes} onChange={(e) => setCanary(e.target.checked)} /> Run the first database alone, then wait
            </label>
          </div>
          {engines.length > 1 ? (
            <div className="callout callout-warning">
              The chosen databases run on different engines ({engines.join(", ")}); the script must work on each.
            </div>
          ) : null}
          {writes ? (
            <div className="callout callout-warning">
              This script writes. It runs with your write access on each database, which is checked again before each one.
            </div>
          ) : null}
          <ErrorText message={error ?? undefined} />
          <div className="flex items-center gap-2" style={{ justifyContent: "flex-end" }}>
            <span className="text-sm muted">
              {n} database{n === 1 ? "" : "s"} selected
            </span>
            <button className="btn btn-primary" onClick={() => void run()} disabled={create.isPending || n === 0}>
              {create.isPending ? <Spinner /> : <Play size={15} />} Run on {n}
            </button>
          </div>
        </section>
        <section className="card panel">
          <h2 className="panel-title">Databases</h2>
          <TargetPicker selected={selected} onChange={setSelected} />
        </section>
      </div>

      <section className="section" style={{ marginTop: "1.5rem" }}>
        <h2 className="section-header-title" style={{ marginBottom: ".6rem" }}>Your recent runs</h2>
        {runs.isLoading ? (
          <Spinner />
        ) : (runs.data ?? []).length === 0 ? (
          <EmptyState title="No fleet runs yet" hint="Runs you start appear here for 30 days." />
        ) : (
          <div className="card">
            <table className="table">
              <thead>
                <tr>
                  <th>Started</th>
                  <th>Script</th>
                  <th>Databases</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {(runs.data ?? []).map((r) => (
                  <tr key={r.id}>
                    <td>
                      <Link className="link" href={`/fleet/${r.id}`}>
                        <Time value={r.created_at} />
                      </Link>
                    </td>
                    <td>
                      <code className="fleet-sql-snippet">{r.sql.slice(0, 90)}</code>
                    </td>
                    <td className="text-sm">
                      {r.succeeded}/{r.total} ok{r.failed ? `, ${r.failed} failed` : ""}
                    </td>
                    <td>
                      <StatusBadge status={r.status} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}
