"use client";

import { ArrowLeftRight, SquareTerminal } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { SqlEditor } from "@/components/sql-editor";
import { Button, Callout, CopyButton, ErrorText, Modal, Select, buttonClass } from "@/components/ui";
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
      <div className="flex flex-col gap-3.5">
        {snapshot ? (
          <p className="text-sm m-0">
            Compares <strong>{database.name}</strong> as it is now with <strong>{snapshot.label}</strong> and writes the
            script that puts its structure back — undoing every structural change made since. Data is not restored: dropped
            tables and columns come back empty.
          </p>
        ) : (
          <>
            <p className="text-sm m-0">
              Compares the structure of <strong>{database.name}</strong> with another {engine ?? ""} database — tables,
              columns, indexes, constraints, views, routines and triggers — and writes the script that makes one match
              the other. Nothing is changed until you run the script.
            </p>
            <div className="flex flex-wrap items-center gap-2">
              <Select className="max-w-88" value={other} onChange={(e) => setOther(e.target.value)} aria-label="Other database">
                <option value="">Choose a database…</option>
                {others.map((d: Database) => (
                  <option key={d.id} value={d.id}>
                    {d.name} · {d.instance?.name ?? d.instance_id.slice(0, 8)}
                  </option>
                ))}
              </Select>
              <Button
                size="sm"
                icon={<ArrowLeftRight size={14} aria-hidden />}
                onClick={() => setDirection((d) => (d === "push" ? "pull" : "push"))}
                disabled={!other}
              >
                Swap
              </Button>
            </div>
            {other ? (
              <p className="text-sm muted m-0">
                The script will change <strong>{targetName}</strong> to match <strong>{sourceName}</strong>.
              </p>
            ) : others.length === 0 && data ? (
              <p className="text-sm muted m-0">
                There is no other {engine ?? ""} database to compare with.
              </p>
            ) : null}
          </>
        )}
        <div>
          <Button size="sm" variant="primary" loading={compare.isPending} disabled={!snapshot && !other} onClick={run}>
            Compare
          </Button>
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
      <Callout tone="success" role="status" className="mb-0">
        The schemas are identical.
      </Callout>
    );
  }
  const n = result.changes.length;
  return (
    <>
      {result.warnings.map((w) => (
        <Callout key={w} tone="warning" className="mb-0">
          {w}
        </Callout>
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
          <div className="flex items-center justify-end gap-2">
            <CopyButton value={result.sql} label="Copy script" className={buttonClass({ size: "sm" })} />
            <Button size="sm" variant="primary" icon={<SquareTerminal size={14} aria-hidden />} onClick={onOpen}>
              Open in SQL console on {targetName}
            </Button>
          </div>
        </>
      ) : null}
    </>
  );
}
