"use client";

import { ArrowLeftRight, SquareTerminal } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { SqlEditor } from "@/components/sql-editor";
import { CopyButton, ErrorText, Modal, Spinner } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { stashQueryDraft } from "@/lib/data-browser/storage";
import { useCompareSchemas, useDatabases } from "@/lib/hooks";
import type { Database, SchemaComparison } from "@/lib/types";

import { SchemaChangeList } from "./schema-changes";

/** A snapshot of this database to compare from instead of its live schema. */
export interface SnapshotSource {
  id: string;
  label: string;
}

/**
 * SchemaCompare compares this database's schema with another database on the
 * same engine (or this database with one of its own snapshots) and shows the
 * script that makes the target match the source. Nothing runs here: the
 * script opens in the SQL console, where it is run with the usual
 * confirmation.
 */
export function SchemaCompare({
  database,
  snapshot,
  onClose,
}: {
  database: Database;
  snapshot?: SnapshotSource;
  onClose: () => void;
}) {
  const router = useRouter();
  const engine = database.instance?.engine;
  const { data } = useDatabases({ enabled: !snapshot });
  const others = useMemo(
    () =>
      (data?.items ?? []).filter(
        (d) => d.id !== database.id && !d.system && d.status !== "missing" && d.instance?.engine === engine,
      ),
    [data, database.id, engine],
  );
  const [other, setOther] = useState("");
  // "push": make the other database match this one; "pull": the reverse.
  const [direction, setDirection] = useState<"push" | "pull">("push");
  const compare = useCompareSchemas();
  const [result, setResult] = useState<{ cmp: SchemaComparison; target: string; targetName: string } | null>(null);
  const [error, setError] = useState<string | null>(null);

  const otherDb = others.find((d) => d.id === other);
  const sourceName = snapshot ? snapshot.label : direction === "push" ? database.name : (otherDb?.name ?? "");
  const targetName = snapshot ? database.name : direction === "push" ? (otherDb?.name ?? "") : database.name;

  const run = () => {
    setError(null);
    setResult(null);
    const req = snapshot
      ? { source_database_id: database.id, source_snapshot_id: snapshot.id, target_database_id: database.id }
      : direction === "push"
        ? { source_database_id: database.id, target_database_id: other }
        : { source_database_id: other, target_database_id: database.id };
    compare.mutate(req, {
      onSuccess: (cmp) => setResult({ cmp, target: req.target_database_id, targetName }),
      onError: (e) => setError(e instanceof ApiError ? e.message : "Comparison failed"),
    });
  };

  const openInConsole = () => {
    if (!result) return;
    stashQueryDraft(result.target, result.cmp.sql);
    router.push(`/data?db=${encodeURIComponent(result.target)}&query=draft`);
  };

  return (
    <Modal open onClose={onClose} title={snapshot ? "Return the schema to an earlier version" : "Compare schemas"} wide>
      <div className="flex" style={{ flexDirection: "column", gap: ".85rem" }}>
        {snapshot ? (
          <p className="text-sm" style={{ margin: 0 }}>
            Compares <strong>{database.name}</strong> as it is now with <strong>{snapshot.label}</strong> and writes the
            script that puts its structure back — undoing every structural change made since. Data is not restored: dropped
            tables and columns come back empty.
          </p>
        ) : (
          <>
            <p className="text-sm" style={{ margin: 0 }}>
              Compares the structure of <strong>{database.name}</strong> with another {engine ?? ""} database — tables,
              columns, indexes, constraints, views, routines and triggers — and writes the script that makes one match
              the other. Nothing is changed until you run the script.
            </p>
            <div className="flex items-center gap-2" style={{ flexWrap: "wrap" }}>
              <select className="input" style={{ maxWidth: "22rem" }} value={other} onChange={(e) => setOther(e.target.value)} aria-label="Other database">
                <option value="">Choose a database…</option>
                {others.map((d: Database) => (
                  <option key={d.id} value={d.id}>
                    {d.name} · {d.instance?.name ?? d.instance_id.slice(0, 8)}
                  </option>
                ))}
              </select>
              <button className="btn btn-sm" onClick={() => setDirection((d) => (d === "push" ? "pull" : "push"))} disabled={!other}>
                <ArrowLeftRight size={14} /> Swap
              </button>
            </div>
            {other ? (
              <p className="text-sm muted" style={{ margin: 0 }}>
                The script will change <strong>{targetName}</strong> to match <strong>{sourceName}</strong>.
              </p>
            ) : others.length === 0 && data ? (
              <p className="text-sm muted" style={{ margin: 0 }}>
                There is no other {engine ?? ""} database to compare with.
              </p>
            ) : null}
          </>
        )}
        <div>
          <button className="btn btn-primary btn-sm" disabled={compare.isPending || (!snapshot && !other)} onClick={run}>
            {compare.isPending ? <Spinner /> : null} Compare
          </button>
        </div>
        <ErrorText message={error ?? undefined} />
        {result ? <CompareResult result={result.cmp} targetName={result.targetName} onOpen={openInConsole} /> : null}
      </div>
    </Modal>
  );
}

function CompareResult({ result, targetName, onOpen }: { result: SchemaComparison; targetName: string; onOpen: () => void }) {
  if (result.same) {
    return (
      <div className="callout callout-info" role="status">
        The schemas are identical.
      </div>
    );
  }
  const n = result.changes.length;
  return (
    <>
      {result.warnings.map((w) => (
        <div key={w} className="callout callout-warning">
          {w}
        </div>
      ))}
      <div className="text-sm">
        <strong>{n}</strong> difference{n === 1 ? "" : "s"}.
        {result.review > 0
          ? ` ${result.review} step${result.review === 1 ? " is" : "s are"} left commented out in the script for you to review (dropping tables or columns loses data, and some changes can't be generated).`
          : ""}
      </div>
      <SchemaChangeList
        changes={result.changes}
        beforeLabel={`In ${targetName} now`}
        afterLabel="Wanted"
        opLabels={{ added: "missing", removed: "extra", changed: "differs" }}
      />
      {result.sql ? (
        <>
          <SqlEditor value={result.sql} onChange={() => {}} readOnly minHeight="12rem" />
          <div className="flex items-center gap-2" style={{ justifyContent: "flex-end" }}>
            <CopyButton value={result.sql} label="Copy script" />
            <button className="btn btn-primary btn-sm" onClick={onOpen}>
              <SquareTerminal size={14} /> Open in SQL console on {targetName}
            </button>
          </div>
        </>
      ) : null}
    </>
  );
}
