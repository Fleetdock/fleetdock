"use client";

import { useState } from "react";

import { History, RotateCcw, X } from "lucide-react";

import { useConfirm } from "@/components/confirm";
import { useToast } from "@/components/toast";
import { Spinner } from "@/components/ui";
import { changedColumns, displayValue, groupByBatch, keyLabel, type ChangeGroup } from "@/lib/data-browser/history";
import { useRowChanges, useUndo } from "@/lib/hooks";
import type { RowChange, UndoResult } from "@/lib/types";

const OP_LABEL: Record<RowChange["op"], string> = { insert: "Added", update: "Edited", delete: "Deleted" };

/**
 * RowHistory is the data browser's change journal for one table: every edit
 * made here in the last days, who made it, the values before and after, and
 * Undo per change or per save. An undo checks first that nobody changed the
 * row since; if someone did, it asks before overwriting.
 */
export function RowHistory({
  databaseId,
  table,
  canWrite,
  onClose,
}: {
  databaseId: string;
  table: string;
  canWrite: boolean;
  onClose: () => void;
}) {
  const { data, isLoading, error } = useRowChanges(databaseId, table);
  const undo = useRowUndo(databaseId);
  const groups = groupByBatch(data ?? []);

  return (
    <aside className="dbx-valuepanel dbx-history" aria-label="Change history">
      <div className="dbx-valuepanel-head">
        <History size={14} aria-hidden />
        <span className="font-medium" style={{ flex: 1 }}>
          Change history
        </span>
        <button className="btn btn-ghost btn-icon" onClick={onClose} aria-label="Close change history">
          <X size={14} />
        </button>
      </div>
      <div className="dbx-valuepanel-body">
        {isLoading ? (
          <div className="flex items-center gap-2 text-sm muted">
            <Spinner /> Loading…
          </div>
        ) : error ? (
          <p className="text-sm muted">{error instanceof Error ? error.message : "Could not load the history."}</p>
        ) : groups.length === 0 ? (
          <p className="text-sm muted">
            No edits to this table yet. Rows you add, edit or delete here are listed for a week and can be undone.
          </p>
        ) : (
          groups.map((g) => <Group key={g.id} group={g} canWrite={canWrite} undo={undo} />)
        )}
      </div>
    </aside>
  );
}

export interface RowUndo {
  run: (input: { changeId?: string; batchId?: string }) => Promise<void>;
  pending: boolean;
}

/**
 * useRowUndo runs an undo and reports it; on a conflict (the row changed
 * since) it asks whether to overwrite and retries with force.
 */
export function useRowUndo(databaseId: string): RowUndo {
  const mutation = useUndo(databaseId);
  const confirm = useConfirm();
  const { push } = useToast();

  const report = (results: UndoResult[]) => {
    const ok = results.filter((r) => r.ok).length;
    const failed = results.filter((r) => !r.ok && !r.conflict);
    if (ok) push("success", `Undid ${ok} change${ok === 1 ? "" : "s"}`);
    for (const f of failed) push("error", f.error ?? "Could not undo");
  };

  const run = async (input: { changeId?: string; batchId?: string }) => {
    try {
      const results = await mutation.mutateAsync(input);
      report(results.filter((r) => !r.conflict));
      const conflicts = results.filter((r) => r.conflict);
      if (conflicts.length === 0) return;
      const yes = await confirm({
        title: "The row changed since",
        message:
          conflicts.length === 1
            ? "Someone changed this row after the edit you are undoing. Undo anyway and overwrite their change?"
            : `${conflicts.length} rows changed after the edits you are undoing. Undo them anyway and overwrite those changes?`,
        confirmLabel: "Undo anyway",
        danger: true,
      });
      if (!yes) return;
      for (const c of conflicts) report(await mutation.mutateAsync({ changeId: c.change_id, force: true }));
    } catch (err) {
      push("error", err instanceof Error ? err.message : "Could not undo");
    }
  };
  return { run, pending: mutation.isPending };
}

function Group({ group, canWrite, undo }: { group: ChangeGroup; canWrite: boolean; undo: RowUndo }) {
  const [open, setOpen] = useState(group.changes.length <= 3);
  const when = new Date(group.at);
  const n = group.changes.length;
  return (
    <section className="dbx-history-group">
      <header className="flex items-center gap-2">
        <span className="text-sm" style={{ flex: 1, minWidth: 0 }}>
          <span className="font-medium">{group.source === "undo" ? "Undo" : n === 1 ? OP_LABEL[group.changes[0].op] : `Saved ${n} changes`}</span>
          <span className="muted">
            {" · "}
            <time dateTime={group.at} title={when.toLocaleString()}>
              {when.toLocaleString(undefined, { dateStyle: "short", timeStyle: "short" })}
            </time>
            {group.user ? ` · ${group.user}` : ""}
          </span>
        </span>
        {canWrite && group.undoable > 0 ? (
          <button
            className="btn btn-sm"
            disabled={undo.pending}
            onClick={() => void undo.run(group.batchId && n > 1 ? { batchId: group.batchId } : { changeId: group.changes[0].id })}
            title={group.source === "undo" ? "Redo: put back what this undo reverted" : n > 1 ? "Undo every change of this save" : "Undo this change"}
          >
            <RotateCcw size={13} /> {group.source === "undo" ? "Redo" : "Undo"}
            {n > 1 ? " all" : ""}
          </button>
        ) : null}
      </header>
      {n > 3 ? (
        <button className="btn btn-ghost btn-sm" onClick={() => setOpen(!open)} style={{ alignSelf: "flex-start" }}>
          {open ? "Hide rows" : `Show ${n} rows`}
        </button>
      ) : null}
      {open ? group.changes.map((c) => <ChangeRow key={c.id} change={c} canWrite={canWrite && n > 1} undo={undo} />) : null}
    </section>
  );
}

function ChangeRow({ change: c, canWrite, undo }: { change: RowChange; canWrite: boolean; undo: RowUndo }) {
  const diffs = changedColumns(c);
  return (
    <div className={`dbx-history-change${c.undone_at ? " is-undone" : ""}`}>
      <div className="flex items-center gap-2">
        <span className={`badge ${c.op === "delete" ? "badge-red" : c.op === "insert" ? "badge-green" : "badge-gray"}`}>{c.op}</span>
        <code className="text-sm truncate" style={{ flex: 1 }} title={keyLabel(c)}>
          {keyLabel(c)}
        </code>
        {c.undone_at ? <span className="text-sm muted">undone</span> : null}
        {canWrite && c.undoable && !c.undone_at ? (
          <button className="btn btn-ghost btn-sm" disabled={undo.pending} onClick={() => void undo.run({ changeId: c.id })} aria-label="Undo this row's change">
            <RotateCcw size={13} />
          </button>
        ) : null}
      </div>
      {!c.undoable && c.note ? <p className="text-sm muted" style={{ margin: 0 }}>{c.note}</p> : null}
      {c.op === "update" && diffs.length ? (
        <dl className="dbx-history-diff">
          {diffs.map((d) => (
            <div key={d.column}>
              <dt>{d.column}</dt>
              <dd>
                <del>{displayValue(d.before)}</del> → <ins>{displayValue(d.after)}</ins>
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
    </div>
  );
}
