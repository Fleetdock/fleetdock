"use client";

import Link from "next/link";
import { useMemo, useState, type FormEvent } from "react";

import { Archive, Lock, Plus, Trash2, Unlock } from "lucide-react";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { DeleteDatabaseModal } from "@/components/delete-database-modal";
import { ErrorText, Field, Modal, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  LIST_PAGE_SIZE,
  useCan,
  useCreateDatabase,
  useDatabases,
  useDestinations,
  useInstances,
  useLockDatabase,
  useTriggerBackup,
  useUnlockDatabase,
} from "@/lib/hooks";
import type { Database, Instance } from "@/lib/types";

export default function DatabasesPage() {
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const { data, isLoading, error } = useDatabases({ search, page });
  const { data: instances } = useInstances();
  const [open, setOpen] = useState(false);
  const [backupTarget, setBackupTarget] = useState<Database | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Database | null>(null);
  const can = useCan();
  const canWrite = can("database:write");
  const canBackup = can("backup:write");
  const showActions = canWrite || canBackup;

  const lock = useLockDatabase();
  const unlock = useUnlockDatabase();

  const instanceName = useMemo(() => {
    const m = new Map<string, string>();
    instances?.items.forEach((i) => m.set(i.id, i.name));
    return m;
  }, [instances]);

  const instanceById = useMemo(() => {
    const m = new Map<string, Instance>();
    instances?.items.forEach((i) => m.set(i.id, i));
    return m;
  }, [instances]);

  const columns = useMemo(() => {
    const cols: DataTableColumn<Database>[] = [
      {
        id: "name",
        header: "Name",
        className: "font-medium",
        render: (d) => <Link href={`/databases/${d.id}`} style={{ textDecoration: "underline" }}>{d.name}</Link>,
      },
      {
        id: "instance",
        header: "Instance",
        className: "muted",
        render: (d) => instanceName.get(d.instance_id) ?? d.instance_id.slice(0, 8),
      },
      { id: "charset", header: "Charset", className: "muted", render: (d) => d.charset },
      { id: "status", header: "Status", render: (d) => <StatusBadge status={d.status} /> },
    ];
    if (showActions) {
      cols.push({
        id: "actions",
        header: "Actions",
        align: "right",
        render: (d) => (
          <div className="flex items-center gap-2" style={{ justifyContent: "flex-end" }}>
            {canBackup ? (
              <button className="btn btn-sm" onClick={() => setBackupTarget(d)} title="Back up to S3/R2">
                <Archive size={15} /> Backup
              </button>
            ) : null}
            {canWrite ? (
              <>
                {d.status === "locked" ? (
                  <button className="btn btn-sm" onClick={() => unlock.mutate(d.id)} disabled={unlock.isPending}>
                    <Unlock size={15} /> Unlock
                  </button>
                ) : (
                  <button className="btn btn-sm" onClick={() => lock.mutate(d.id)} disabled={lock.isPending}>
                    <Lock size={15} /> Lock
                  </button>
                )}
                <button className="btn btn-sm btn-danger" onClick={() => setDeleteTarget(d)} aria-label="Delete">
                  <Trash2 size={15} />
                </button>
              </>
            ) : null}
          </div>
        ),
      });
    }
    return cols;
  }, [canBackup, canWrite, instanceName, lock.isPending, showActions, unlock.isPending]);

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">Databases</h1>
          <p className="muted text-sm">Logical databases across your instances.</p>
        </div>
        {canWrite ? (
          <button className="btn btn-primary" onClick={() => setOpen(true)}>
            <Plus size={16} /> Create database
          </button>
        ) : null}
      </div>

      <DataTable<Database>
        columns={columns}
        rows={data?.items ?? []}
        rowKey={(d) => d.id}
        isLoading={isLoading}
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not load databases"
        emptyTitle="No databases yet"
        emptyHint="Create your first database to get started."
        emptySearchTitle="No databases match your search"
        search={{
          value: search,
          onChange: (v) => {
            setSearch(v);
            setPage(1);
          },
          placeholder: "Search databases…",
        }}
        pagination={{
          page,
          pageCount: Math.max(1, Math.ceil((data?.pagination.total ?? 0) / LIST_PAGE_SIZE)),
          onPage: setPage,
        }}
      />

      <CreateDatabaseModal open={open} onClose={() => setOpen(false)} instances={instances?.items ?? []} />
      <BackupDatabaseModal database={backupTarget} onClose={() => setBackupTarget(null)} />
      <DeleteDatabaseModal
        database={deleteTarget}
        instance={deleteTarget ? instanceById.get(deleteTarget.instance_id) : undefined}
        onClose={() => setDeleteTarget(null)}
      />
    </div>
  );
}

function BackupDatabaseModal({ database, onClose }: { database: Database | null; onClose: () => void }) {
  const trigger = useTriggerBackup();
  const { data: destinations } = useDestinations();
  const [destinationId, setDestinationId] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await trigger.mutateAsync({ database_id: database!.id, destination_id: destinationId });
      setDone(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to start backup");
    }
  }

  function close() {
    setDestinationId("");
    setDone(false);
    setError(null);
    onClose();
  }

  if (!database) return null;
  return (
    <Modal open onClose={close} title={`Back up "${database.name}"`}>
      {done ? (
        <div>
          <p className="text-sm">Backup started — track it on the Backups page.</p>
          <div className="flex items-center justify-end" style={{ marginTop: ".8rem" }}>
            <button className="btn btn-primary" onClick={close}>Done</button>
          </div>
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          <Field label="Destination">
            <select className="input" value={destinationId} onChange={(e) => setDestinationId(e.target.value)} required>
              <option value="" disabled>Select an S3/R2 destination…</option>
              {destinations?.items.map((d) => (
                <option key={d.id} value={d.id}>{d.name} ({d.bucket})</option>
              ))}
            </select>
          </Field>
          {!destinations || destinations.items.length === 0 ? (
            <p className="muted text-sm">No destinations yet — add one on the Destinations page.</p>
          ) : null}
          <ErrorText message={error ?? undefined} />
          <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
            <button type="button" className="btn" onClick={close}>Cancel</button>
            <button
              type="submit"
              className="btn btn-primary"
              disabled={trigger.isPending || !destinations || destinations.items.length === 0}
            >
              {trigger.isPending ? "Starting…" : "Start backup"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}

function CreateDatabaseModal({
  open,
  onClose,
  instances,
}: {
  open: boolean;
  onClose: () => void;
  instances: { id: string; name: string }[];
}) {
  const create = useCreateDatabase();
  const [instanceId, setInstanceId] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await create.mutateAsync({ instance_id: instanceId, name });
      setName("");
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create database");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Create database">
      <form onSubmit={onSubmit}>
        <Field label="Instance">
          <select className="input" value={instanceId} onChange={(e) => setInstanceId(e.target.value)} required>
            <option value="" disabled>
              Select an instance…
            </option>
            {instances.map((i) => (
              <option key={i.id} value={i.id}>
                {i.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="app_production" required />
        </Field>
        {instances.length === 0 ? (
          <p className="muted text-sm">Register a server and add an instance first.</p>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={create.isPending || instances.length === 0}>
            {create.isPending ? "Creating…" : "Create"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
