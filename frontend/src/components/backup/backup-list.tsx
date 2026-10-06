"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { Download, ShieldCheck, Trash2, Upload } from "lucide-react";

import { DataTable, type DataTableColumn } from "@/components/data-table";
import { useErrorToast, useToast } from "@/components/toast";
import { ConfirmModal, ErrorText, Field, Menu, Modal, StatusBadge, Time } from "@/components/ui";
import { friendlyError } from "@/lib/errors";
import { formatBytes } from "@/lib/format";
import {
  LIST_PAGE_SIZE,
  downloadBackup,
  useBackupActions,
  useBackups,
  useCan,
  useInstances,
  useRestoreBackup,
} from "@/lib/hooks";
import type { Backup } from "@/lib/types";

/**
 * BackupList is the backup history table with its row actions (restore,
 * download, check, delete). Pass databaseId to show one database's backups.
 */
export function BackupList({
  databaseId,
  search,
  emptyHint,
  emptyAction,
}: {
  databaseId?: string;
  search?: string;
  emptyHint?: string;
  emptyAction?: React.ReactNode;
}) {
  const [page, setPage] = useState(1);
  const { data, isLoading, error } = useBackups(databaseId, page, search);
  const can = useCan();
  const canWrite = can("backup:write");
  const actions = useBackupActions();
  const toastError = useErrorToast();
  const { push } = useToast();
  const [restoreTarget, setRestoreTarget] = useState<Backup | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Backup | null>(null);

  async function download(b: Backup) {
    try {
      await downloadBackup(b.id);
    } catch (err) {
      push("error", friendlyError(err, "The download failed"));
    }
  }

  const columns: DataTableColumn<Backup>[] = [
    ...(databaseId
      ? []
      : [
          {
            id: "database",
            header: "Database",
            className: "font-medium",
            render: (b: Backup) => (
              <span className="flex flex-col">
                <Link href={`/databases/${b.database_id}`} className="link-plain">
                  {b.database_name || "Removed database"}
                </Link>
                {b.instance_name ? <span className="muted text-sm">{b.instance_name}</span> : null}
              </span>
            ),
          },
        ]),
    {
      id: "created",
      header: "Taken",
      className: databaseId ? "font-medium" : "muted",
      render: (b) => (
        <span className="flex flex-col">
          <Time value={b.created_at} />
          {b.type === "scheduled" ? <span className="muted text-sm">automatic</span> : null}
        </span>
      ),
    },
    {
      id: "status",
      header: "Status",
      render: (b) => (
        <>
          <StatusBadge status={b.status} />
          {b.error ? (
            <div className="muted text-sm truncate" style={{ maxWidth: 280 }} title={b.error}>
              {b.error}
            </div>
          ) : null}
        </>
      ),
    },
    {
      id: "size",
      header: "Size",
      align: "right",
      className: "muted",
      hideOnMobile: true,
      render: (b) => formatBytes(b.size_bytes),
    },
    {
      id: "verified",
      header: "Checked",
      hideOnMobile: true,
      render: (b) => <VerifyBadge backup={b} />,
    },
    {
      id: "actions",
      header: "",
      align: "right",
      render: (b) => {
        const completed = b.status === "completed";
        return (
          <Menu
            label="Backup actions"
            items={[
              {
                label: "Restore…",
                icon: <Upload size={15} />,
                onSelect: () => setRestoreTarget(b),
                hidden: !canWrite || !completed,
              },
              {
                label: "Download",
                icon: <Download size={15} />,
                onSelect: () => void download(b),
                hidden: !canWrite || !completed,
              },
              {
                label: "Check that it restores",
                icon: <ShieldCheck size={15} />,
                onSelect: () =>
                  actions.verify.mutate(b.id, {
                    onSuccess: () => push("info", "Checking the backup — it is restored into a temporary database"),
                    onError: toastError("Could not start the check"),
                  }),
                hidden: !canWrite || !completed,
                disabled: b.verify_status === "running",
              },
              {
                label: "Delete…",
                icon: <Trash2 size={15} />,
                danger: true,
                onSelect: () => setDeleteTarget(b),
                hidden: !canWrite || b.status === "pending" || b.status === "running" || b.status === "deleted",
              },
            ]}
          />
        );
      },
    },
  ];

  return (
    <>
      <DataTable<Backup>
        columns={columns}
        rows={data?.items ?? []}
        rowKey={(b) => b.id}
        isLoading={isLoading}
        error={error ? friendlyError(error) : undefined}
        errorTitle="Could not load backups"
        emptyTitle={search ? "No backups match your search" : "No backups yet"}
        emptyHint={search ? undefined : emptyHint}
        emptyAction={search ? undefined : emptyAction}
        pagination={{
          page,
          pageCount: Math.max(1, Math.ceil((data?.pagination.total ?? 0) / LIST_PAGE_SIZE)),
          onPage: setPage,
        }}
      />
      <ConfirmModal
        open={deleteTarget !== null}
        danger
        title="Delete this backup?"
        confirmLabel="Delete backup"
        busy={actions.remove.isPending}
        message="The backup file is permanently deleted from storage and can no longer be restored."
        onConfirm={async () => {
          if (!deleteTarget) return;
          try {
            await actions.remove.mutateAsync(deleteTarget.id);
            push("success", "Backup deleted");
          } catch (err) {
            push("error", friendlyError(err, "Failed to delete the backup"));
          }
          setDeleteTarget(null);
        }}
        onCancel={() => setDeleteTarget(null)}
      />
      <RestoreModal backup={restoreTarget} onClose={() => setRestoreTarget(null)} />
    </>
  );
}

function RestoreModal({ backup, onClose }: { backup: Backup | null; onClose: () => void }) {
  const restore = useRestoreBackup();
  const { data: instances } = useInstances();
  const [targetInstance, setTargetInstance] = useState("");
  const [targetDatabase, setTargetDatabase] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [operationId, setOperationId] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      const res = await restore.mutateAsync({
        backup_id: backup!.id,
        target_instance_id: targetInstance || undefined,
        target_database: targetDatabase || undefined,
      });
      setOperationId(res.operation_id ?? "");
    } catch (err) {
      setError(err);
    }
  }

  function close() {
    setTargetInstance("");
    setTargetDatabase("");
    setOperationId(null);
    setError(null);
    onClose();
  }

  if (!backup) return null;
  const name = backup.database_name || "database";
  return (
    <Modal open onClose={close} title={`Restore ${name}`}>
      {operationId !== null ? (
        <div>
          <p className="text-sm" style={{ marginTop: 0 }}>
            The restore has started. It runs in the background — you&apos;ll get a notification when it finishes.
          </p>
          <div className="flex items-center justify-end gap-2" style={{ marginTop: ".8rem" }}>
            <Link href={operationId ? `/activity/${operationId}` : "/activity"} className="btn" onClick={close}>
              Follow progress
            </Link>
            <button className="btn btn-primary" onClick={close}>
              Done
            </button>
          </div>
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          <div className="callout callout-warning">
            Restoring into the original database <strong>replaces its current contents</strong> with this backup
            {backup.completed_at ? (
              <>
                {" "}
                from <Time value={backup.completed_at} />
              </>
            ) : null}
            . To keep both, enter a new database name below.
          </div>
          <Field label="Database server" hint="Pick another server to copy the database there.">
            <select className="input" value={targetInstance} onChange={(e) => setTargetInstance(e.target.value)}>
              <option value="">{backup.instance_name ? `${backup.instance_name} (original)` : "Original server"}</option>
              {instances?.items
                .filter((i) => i.has_credentials)
                .map((i) => (
                  <option key={i.id} value={i.id}>
                    {i.name} ({i.kind === "external" ? `${i.host}:${i.port}` : `port ${i.port}`})
                  </option>
                ))}
            </select>
          </Field>
          <Field label="Database name" hint="Leave empty to restore under the original name.">
            <input
              className="input"
              value={targetDatabase}
              onChange={(e) => setTargetDatabase(e.target.value)}
              placeholder={backup.database_name || "original name"}
            />
          </Field>
          <ErrorText message={error ? friendlyError(error, "Failed to start the restore") : undefined} />
          <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
            <button type="button" className="btn" onClick={close}>
              Cancel
            </button>
            <button type="submit" className="btn btn-primary" disabled={restore.isPending}>
              {restore.isPending ? "Starting…" : "Restore"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}

/** VerifyBadge shows the outcome of the latest test restore ("check"). */
export function VerifyBadge({ backup }: { backup: Backup }) {
  switch (backup.verify_status) {
    case "passed":
      return (
        <span className="badge badge-green" title="A test restore of this backup succeeded">
          restores OK
        </span>
      );
    case "failed":
      return (
        <span className="badge badge-red" title={backup.verify_error ?? "The test restore failed"}>
          check failed
        </span>
      );
    case "running":
      return <span className="badge badge-gray">checking…</span>;
    default:
      return <span className="muted text-sm">not checked</span>;
  }
}
