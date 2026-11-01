"use client";

import Link from "next/link";
import { useMemo, useRef, useState } from "react";

import { History, RotateCcw, Trash2 } from "lucide-react";

import { useConfirm } from "@/components/confirm";
import { useToast } from "@/components/toast";
import { Badge, Button, Callout, ErrorText, Modal, SegmentedControl, Spinner, type Tone } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { displayValue } from "@/lib/data-browser/history";
import { formatBytes } from "@/lib/format";
import { diffCopy, useBackupCopies, useBackupCopyMutations, useBackups } from "@/lib/hooks";
import type { BackupCopy, CopyDiff, DiffCursor, DiffItem } from "@/lib/types";

import { useRowUndo } from "./row-history";

type Kind = DiffItem["kind"];
const KIND_LABEL: Record<Kind, string> = { changed: "Changed", removed: "Deleted since", added: "Added since" };
const KIND_BADGE: Record<Kind, Tone> = { changed: "warning", removed: "danger", added: "success" };

/** How many differences one "Compare" collects before pausing. */
const COLLECT = 200;

const keyText = (k: DiffItem["key"]) =>
  Object.entries(k)
    .map(([c, v]) => `${c} = ${displayValue(v)}`)
    .join(", ");

/**
 * BackupCompare compares a table with one of its backups and puts chosen
 * rows back. The backup is restored next to the database first (a backup
 * copy, kept for a couple of hours); then the table is compared row by row
 * by its key. Restoring is journaled, so it can be undone from the change
 * history.
 */
export function BackupCompare({
  databaseId,
  table,
  canWrite,
  canBackup,
  onClose,
}: {
  databaseId: string;
  table: string;
  canWrite: boolean;
  canBackup: boolean;
  onClose: () => void;
}) {
  const copies = useBackupCopies(databaseId);
  const live = (copies.data ?? []).find((c) => c.status === "restoring" || c.status === "ready");
  const failed = (copies.data ?? []).find((c) => c.status === "failed");

  return (
    <Modal open onClose={onClose} title={`Compare ${table} with a backup`} wide>
      {copies.isLoading ? (
        <div className="flex items-center gap-2 text-sm muted">
          <Spinner /> Loading…
        </div>
      ) : live?.status === "ready" ? (
        <ReadyCopy databaseId={databaseId} table={table} copy={live} canWrite={canWrite} canBackup={canBackup} />
      ) : live ? (
        <Callout
          tone="info"
          icon={false}
          actions={
            live.job_id ? (
              <Link className="link" href={`/activity/${live.job_id}`}>
                Progress
              </Link>
            ) : undefined
          }
        >
          <span className="flex items-center gap-2">
            <Spinner /> Restoring the backup from {when(live.backup_created_at)} next to the database… This takes about as
            long as restoring it would.
          </span>
        </Callout>
      ) : (
        <PickBackup databaseId={databaseId} canBackup={canBackup} lastError={failed?.error} />
      )}
    </Modal>
  );
}

function when(iso?: string) {
  return iso ? new Date(iso).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" }) : "—";
}

function PickBackup({ databaseId, canBackup, lastError }: { databaseId: string; canBackup: boolean; lastError?: string }) {
  const { data, isLoading } = useBackups(databaseId);
  const { create } = useBackupCopyMutations(databaseId);
  const [error, setError] = useState<string | null>(null);
  const backups = (data?.items ?? []).filter((b) => b.status === "completed" && b.storage_url);

  if (!canBackup) {
    return <p className="text-sm muted">Opening a backup needs the backup:write permission on this database.</p>;
  }
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm m-0">
        Pick a backup. Fleetdock restores it into a temporary database on the same server (it is dropped again after a
        couple of hours), then compares this table with it row by row.
      </p>
      {lastError ? <ErrorText message={`The last attempt failed: ${lastError}`} /> : null}
      <ErrorText message={error ?? undefined} />
      {isLoading ? (
        <Spinner />
      ) : backups.length === 0 ? (
        <p className="text-sm muted">This database has no completed backups in backup storage yet.</p>
      ) : (
        <div className="card max-h-[50vh] overflow-y-auto">
          <table className="table">
            <tbody>
              {backups.map((b) => (
                <tr key={b.id}>
                  <td>
                    <div className="font-medium">{when(b.completed_at ?? b.created_at)}</div>
                    <div className="text-sm muted">
                      {b.type} · {formatBytes(b.size_bytes)}
                      {b.verify_status === "passed" ? " · verified" : ""}
                    </div>
                  </td>
                  <td className="text-right">
                    <Button
                      size="sm"
                      variant="primary"
                      icon={<History size={14} aria-hidden />}
                      disabled={create.isPending}
                      onClick={() =>
                        create.mutate(b.id, { onError: (e) => setError(e instanceof ApiError ? e.message : "Could not open the backup") })
                      }
                    >
                      Open
                    </Button>
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

function ReadyCopy({
  databaseId,
  table,
  copy,
  canWrite,
  canBackup,
}: {
  databaseId: string;
  table: string;
  copy: BackupCopy;
  canWrite: boolean;
  canBackup: boolean;
}) {
  const { extend, discard, restoreRows } = useBackupCopyMutations(databaseId);
  const confirm = useConfirm();
  const { push } = useToast();
  const rowUndo = useRowUndo(databaseId);
  const [diff, setDiff] = useState<{ columns: string[]; items: DiffItem[]; scanned: number; warnings: string[] } | null>(null);
  const [next, setNext] = useState<DiffCursor | null | undefined>(undefined);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<Kind | "all">("all");
  const [picked, setPicked] = useState<Set<number>>(new Set());
  const stop = useRef(false);

  // Collects differences page by page until COLLECT more are found or the
  // comparison ends.
  async function compare(from?: DiffCursor, reset = false) {
    setRunning(true);
    setError(null);
    stop.current = false;
    let cur: DiffCursor | undefined = from;
    let acc = reset || !diff ? { columns: [] as string[], items: [] as DiffItem[], scanned: 0, warnings: [] as string[] } : { ...diff };
    const target = acc.items.length + COLLECT;
    try {
      for (;;) {
        const step: CopyDiff = await diffCopy(databaseId, copy.id, table, cur);
        acc = {
          columns: step.columns.length ? step.columns : acc.columns,
          items: [...acc.items, ...step.items],
          scanned: acc.scanned + step.scanned,
          warnings: Array.from(new Set([...acc.warnings, ...(step.warnings ?? [])])),
        };
        setDiff(acc);
        cur = step.next;
        if (!cur || acc.items.length >= target || stop.current) break;
      }
      setNext(cur ?? null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "The comparison failed");
    } finally {
      setRunning(false);
    }
  }

  const shown = useMemo(
    () => (diff?.items ?? []).map((it, i) => ({ it, i })).filter(({ it }) => filter === "all" || it.kind === filter),
    [diff, filter],
  );
  const counts = useMemo(() => {
    const c: Record<Kind, number> = { added: 0, removed: 0, changed: 0 };
    for (const it of diff?.items ?? []) c[it.kind]++;
    return c;
  }, [diff]);

  async function restore() {
    const rows = [...picked].map((i) => diff!.items[i]);
    const deletes = rows.filter((r) => r.kind === "added").length;
    const ok = await confirm({
      title: `Put ${rows.length} row${rows.length === 1 ? "" : "s"} back as in the backup?`,
      message: `Changed rows get the backup's values and deleted rows come back${
        deletes ? `; ${deletes} row${deletes === 1 ? "" : "s"} added since the backup will be DELETED` : ""
      }. You can undo this from the table's change history.`,
      confirmLabel: deletes ? `Restore and delete ${deletes}` : "Restore",
      danger: deletes > 0,
    });
    if (!ok) return;
    try {
      const res = await restoreRows.mutateAsync({ copyId: copy.id, table, rows });
      const done = res.items.filter((r) => r.ok).length;
      const conflicts = res.items.filter((r) => r.conflict).length;
      const failed = res.items.length - done - conflicts;
      push(
        done ? "success" : "error",
        `Restored ${done} of ${res.items.length} row${res.items.length === 1 ? "" : "s"}` +
          (conflicts ? `; ${conflicts} changed since the comparison (compare again)` : "") +
          (failed ? `; ${failed} failed` : ""),
        done ? { label: "Undo", onClick: () => void rowUndo.run({ batchId: res.batch_id }) } : undefined,
      );
      setPicked(new Set());
      void compare(undefined, true);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Restore failed");
    }
  }

  const toggle = (i: number) =>
    setPicked((p) => {
      const n = new Set(p);
      if (n.has(i)) n.delete(i);
      else n.add(i);
      return n;
    });

  return (
    <div className="flex flex-col gap-3">
      <Callout
        tone="info"
        icon={false}
        className="backup-copy-banner"
        actions={
          canBackup ? (
            <>
              <Button size="sm" disabled={extend.isPending} onClick={() => extend.mutate(copy.id)}>
                Keep longer
              </Button>
              <Button
                size="sm"
                variant="danger"
                icon={<Trash2 size={13} aria-hidden />}
                disabled={discard.isPending}
                onClick={() => discard.mutate(copy.id)}
              >
                Discard
              </Button>
            </>
          ) : undefined
        }
      >
        <span>
          Backup from <strong>{when(copy.backup_created_at)}</strong>, kept until {when(copy.expires_at)}.
        </span>
      </Callout>

      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="primary" loading={running} onClick={() => void compare(undefined, true)}>
          {diff ? "Compare again" : `Compare ${table}`}
        </Button>
        {running ? (
          <Button size="sm" onClick={() => (stop.current = true)}>
            Stop
          </Button>
        ) : null}
        {next && !running ? (
          <Button size="sm" onClick={() => void compare(next)}>
            Find more
          </Button>
        ) : null}
        {diff ? (
          <span className="text-sm muted">
            {diff.scanned.toLocaleString()} rows checked{next === null ? ", done" : running ? "…" : ", more to check"}
          </span>
        ) : null}
      </div>
      <ErrorText message={error ?? undefined} />
      {diff?.warnings.map((w) => (
        <p key={w} className="text-sm muted m-0">
          {w}
        </p>
      ))}

      {diff ? (
        diff.items.length === 0 && next === null ? (
          <p className="text-sm">No differences: the table matches the backup.</p>
        ) : (
          <>
            <SegmentedControl
              label="Show"
              value={filter}
              onChange={setFilter}
              className="self-start"
              options={(["all", "changed", "removed", "added"] as const).map((k) => ({
                value: k,
                label: k === "all" ? `All ${diff.items.length}` : `${KIND_LABEL[k]} ${counts[k]}`,
              }))}
            />
            <div className="card max-h-[45vh] overflow-auto">
              <table className="table compare-table">
                <thead>
                  <tr>
                    {canWrite ? <th className="w-8" /> : null}
                    <th>Row</th>
                    <th>What differs</th>
                  </tr>
                </thead>
                <tbody>
                  {shown.map(({ it, i }) => (
                    <tr key={i}>
                      {canWrite ? (
                        <td>
                          <input type="checkbox" checked={picked.has(i)} onChange={() => toggle(i)} aria-label={`Select ${keyText(it.key)}`} />
                        </td>
                      ) : null}
                      <td>
                        <Badge tone={KIND_BADGE[it.kind]}>{KIND_LABEL[it.kind]}</Badge>
                        <div className="text-sm mt-1">
                          <code>{keyText(it.key)}</code>
                        </div>
                      </td>
                      <td>
                        <DiffCells item={it} columns={diff.columns} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {canWrite ? (
              <div className="flex items-center gap-2">
                <Button size="sm" onClick={() => setPicked(new Set(shown.map(({ i }) => i)))}>
                  Select shown
                </Button>
                <Button size="sm" disabled={picked.size === 0} onClick={() => setPicked(new Set())}>
                  Clear
                </Button>
                <span className="flex-1" />
                <Button
                  size="sm"
                  variant="primary"
                  icon={<RotateCcw size={14} aria-hidden />}
                  disabled={picked.size === 0 || restoreRows.isPending || picked.size > 200}
                  onClick={() => void restore()}
                >
                  Restore {picked.size || ""} from backup
                </Button>
              </div>
            ) : null}
          </>
        )
      ) : null}
    </div>
  );
}

function DiffCells({ item, columns }: { item: DiffItem; columns: string[] }) {
  const cols = item.kind === "changed" ? (item.changed ?? []) : columns.slice(0, 6);
  return (
    <dl className="dbx-history-diff">
      {cols.map((c) => {
        const i = columns.indexOf(c);
        const b = item.backup?.[i];
        const l = item.live?.[i];
        return (
          <div key={c}>
            <dt>{c}</dt>
            <dd>
              {item.kind === "changed" ? (
                <>
                  <span className="muted">backup</span> <ins>{displayValue(b ?? null)}</ins> · <span className="muted">now</span>{" "}
                  <del>{displayValue(l ?? null)}</del>
                </>
              ) : (
                displayValue((item.kind === "removed" ? b : l) ?? null)
              )}
            </dd>
          </div>
        );
      })}
      {item.kind !== "changed" && columns.length > 6 ? <div className="muted">…</div> : null}
    </dl>
  );
}
