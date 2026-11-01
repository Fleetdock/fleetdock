"use client";

import { Camera, GitCompareArrows, History, RotateCcw } from "lucide-react";
import { useState } from "react";

import { Badge, Button, Callout, EmptyState, ErrorText, QueryError, Skeleton, Time } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useSchemaHistory, useSchemaHistoryMutations, useSchemaSnapshot } from "@/lib/hooks";
import type { Database, SchemaSnapshot } from "@/lib/types";

import { SchemaChangeList } from "./schema-changes";
import { SchemaCompare, type SnapshotSource } from "./schema-compare";

const TRIGGER_LABEL: Record<SchemaSnapshot["trigger"], string> = {
  scheduled: "detected",
  manual: "snapshot",
  ddl: "via Fleetdock",
};

function every(seconds: number) {
  if (seconds <= 0) return "";
  if (seconds % 3600 === 0) return seconds === 3600 ? "every hour" : `every ${seconds / 3600} hours`;
  if (seconds % 60 === 0) return seconds === 60 ? "every minute" : `every ${seconds / 60} minutes`;
  return `every ${seconds} s`;
}

function summaryText(s: SchemaSnapshot) {
  const c = s.summary.counts;
  return `${c.tables} table${c.tables === 1 ? "" : "s"}, ${c.views} view${c.views === 1 ? "" : "s"}, ${c.routines} routine${c.routines === 1 ? "" : "s"}, ${c.triggers} trigger${c.triggers === 1 ? "" : "s"}`;
}

/**
 * SchemaHistorySection is the database page's Schema tab: whether the
 * structure is watched, a timeline of every structure it had (each with what
 * changed), a way back to an earlier one, and comparison with another
 * database.
 */
export function SchemaHistorySection({ database, canWrite }: { database: Database; canWrite: boolean }) {
  const { data, isLoading, error, refetch } = useSchemaHistory(database.id);
  const { setWatch, snapshot } = useSchemaHistoryMutations(database.id);
  const [open, setOpen] = useState<string | null>(null);
  const [compare, setCompare] = useState<{ snapshot?: SnapshotSource } | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);

  if (isLoading) {
    return (
      <div role="status" aria-label="Loading schema history" className="stack-sm">
        <Skeleton height="4rem" radius="var(--radius)" />
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} height="2.75rem" width={`${90 - i * 8}%`} radius="var(--radius-md)" />
        ))}
      </div>
    );
  }
  if (error) {
    return <QueryError error={error} what="schema history" onRetry={() => void refetch()} />;
  }
  if (!data) return null;
  const watching = data.watch?.enabled ?? false;
  const fail = (e: unknown) => setActionError(e instanceof ApiError ? e.message : "Something went wrong");
  const takeSnapshot = () => {
    setActionError(null);
    setNote(null);
    snapshot.mutate(undefined, {
      onError: fail,
      onSuccess: (r) => setNote(r.created ? "Snapshot recorded." : "No change since the last snapshot."),
    });
  };

  return (
    <div>
      <div className="card flex flex-wrap items-center gap-3 px-4 py-3 mb-3.5">
        <History size={18} className="muted" aria-hidden />
        <div className="flex-1 min-w-48">
          {watching ? (
            <>
              <div className="font-medium">Schema changes are tracked</div>
              <div className="muted text-sm">
                {data.interval_seconds > 0 ? `Checked ${every(data.interval_seconds)} and after changes made here` : "Checked after changes made here"}
                {data.watch?.last_checked_at ? (
                  <>
                    {" "}· last checked <Time value={data.watch.last_checked_at} />
                  </>
                ) : null}
                . You are notified when it changes.
              </div>
            </>
          ) : (
            <>
              <div className="font-medium">Schema changes are not tracked</div>
              <div className="muted text-sm">
                Record every change to this database&apos;s tables, columns, indexes, views and routines — whoever makes
                it — and get notified.
              </div>
            </>
          )}
        </div>
        {canWrite ? (
          <Button
            size="sm"
            variant={watching ? "secondary" : "primary"}
            loading={setWatch.isPending}
            onClick={() => {
              setActionError(null);
              setWatch.mutate(!watching, { onError: fail });
            }}
          >
            {watching ? "Stop tracking" : "Track changes"}
          </Button>
        ) : null}
        <Button size="sm" icon={<Camera size={14} aria-hidden />} loading={snapshot.isPending} onClick={takeSnapshot}>
          Snapshot now
        </Button>
        <Button size="sm" icon={<GitCompareArrows size={14} aria-hidden />} onClick={() => setCompare({})}>
          Compare with…
        </Button>
      </div>
      <ErrorText message={actionError ?? undefined} />
      {note ? (
        <p className="text-sm muted" role="status">
          {note}
        </p>
      ) : null}
      {data.watch?.last_error ? (
        <Callout tone="warning">The last check failed: {data.watch.last_error}</Callout>
      ) : null}

      {data.items.length === 0 ? (
        <EmptyState
          title="No snapshots yet"
          hint="Track changes or take a snapshot to start this database's schema history."
          action={
            <Button
              variant="primary"
              icon={<Camera size={14} aria-hidden />}
              loading={snapshot.isPending}
              onClick={takeSnapshot}
            >
              Snapshot now
            </Button>
          }
        />
      ) : (
        <ol className="schema-timeline">
          {data.items.map((s) => (
            <SnapshotRow
              key={s.id}
              databaseId={database.id}
              snap={s}
              open={open === s.id}
              onToggle={() => setOpen((o) => (o === s.id ? null : s.id))}
              onRestore={(source) => setCompare({ snapshot: source })}
            />
          ))}
        </ol>
      )}
      {compare ? <SchemaCompare database={database} snapshot={compare.snapshot} onClose={() => setCompare(null)} /> : null}
    </div>
  );
}

function SnapshotRow({
  databaseId,
  snap,
  open,
  onToggle,
  onRestore,
}: {
  databaseId: string;
  snap: SchemaSnapshot;
  open: boolean;
  onToggle: () => void;
  onRestore: (source: SnapshotSource) => void;
}) {
  const t = snap.summary.changes;
  const first = !snap.previous_id;
  return (
    <li className={`schema-snapshot${open ? " is-open" : ""}`}>
      <button type="button" className="schema-snapshot-head" onClick={onToggle} aria-expanded={open}>
        <span className="schema-snapshot-time">
          <Time value={snap.taken_at} />
        </span>
        <Badge>{first ? "first snapshot" : TRIGGER_LABEL[snap.trigger]}</Badge>
        <span className="schema-snapshot-text">
          {first ? summaryText(snap) : (snap.summary.highlights ?? []).join(" · ")}
        </span>
        {!first ? (
          <span className="text-sm muted schema-snapshot-tally">
            {t.added ? `+${t.added} ` : ""}
            {t.removed ? `−${t.removed} ` : ""}
            {t.changed ? `~${t.changed}` : ""}
          </span>
        ) : null}
      </button>
      {open ? <SnapshotDetail databaseId={databaseId} snap={snap} onRestore={onRestore} /> : null}
    </li>
  );
}

function SnapshotDetail({
  databaseId,
  snap,
  onRestore,
}: {
  databaseId: string;
  snap: SchemaSnapshot;
  onRestore: (source: SnapshotSource) => void;
}) {
  const { data, isLoading } = useSchemaSnapshot(databaseId, snap.id);
  const prev = data?.previous;
  return (
    <div className="schema-snapshot-body">
      <p className="text-sm muted m-0">
        {summaryText(snap)}
        {snap.summary.truncated ? " (only the first objects were read)" : ""} · fingerprint <code>{snap.hash.slice(0, 12)}</code>
      </p>
      {!snap.summary.stored ? (
        <p className="text-sm muted">This structure was too large to keep; only its fingerprint was recorded.</p>
      ) : isLoading ? (
        <div role="status" aria-label="Loading changes" className="stack-sm">
          <Skeleton width="60%" />
          <Skeleton width="45%" />
        </div>
      ) : data && snap.previous_id ? (
        <SchemaChangeList changes={data.changes} />
      ) : null}
      {prev?.summary.stored ? (
        <div>
          <Button
            size="sm"
            icon={<RotateCcw size={14} aria-hidden />}
            onClick={() =>
              onRestore({ id: prev.id, label: `its schema as of ${new Date(prev.taken_at).toLocaleString()}` })
            }
          >
            Go back to the version before this…
          </Button>
        </div>
      ) : null}
    </div>
  );
}
