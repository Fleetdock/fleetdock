"use client";

import Link from "next/link";
import { useMemo, useState, type FormEvent } from "react";

import { Archive, Lock, Plus, Search, Trash2, Unlock } from "lucide-react";
import { DeleteDatabaseModal } from "@/components/delete-database-modal";
import { EmptyState, ErrorText, Field, Modal, Pagination, Spinner, StatusBadge } from "@/components/ui";
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

      <div className="flex items-center gap-2" style={{ marginBottom: ".9rem", maxWidth: "22rem" }}>
        <div style={{ position: "relative", width: "100%" }}>
          <span style={{ position: "absolute", left: 10, top: 9, color: "var(--muted)" }}>
            <Search size={16} />
          </span>
          <input
            className="input"
            style={{ paddingLeft: "2rem" }}
            placeholder="Search databases…"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
          />
        </div>
      </div>

      {isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>
      ) : error ? (
        <EmptyState title="Could not load databases" hint={(error as ApiError).message} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState title="No databases yet" hint="Create your first database to get started." />
      ) : (
        <div className="card" style={{ overflow: "hidden" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Instance</th>
                <th>Charset</th>
                <th>Status</th>
                {showActions ? <th style={{ textAlign: "right" }}>Actions</th> : null}
              </tr>
            </thead>
            <tbody>
              {data.items.map((d) => (
                <tr key={d.id}>
                  <td className="font-medium">
                    <Link href={`/databases/${d.id}`} style={{ textDecoration: "underline" }}>{d.name}</Link>
                  </td>
                  <td className="muted">{instanceName.get(d.instance_id) ?? d.instance_id.slice(0, 8)}</td>
                  <td className="muted">{d.charset}</td>
                  <td><StatusBadge status={d.status} /></td>
                  {showActions ? (
                    <td style={{ textAlign: "right" }}>
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
                            <button
                              className="btn btn-sm btn-danger"
                              onClick={() => setDeleteTarget(d)}
                              aria-label="Delete"
                            >
                              <Trash2 size={15} />
                            </button>
                          </>
                        ) : null}
                      </div>
                    </td>
                  ) : null}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {data ? (
        <div className="flex items-center justify-end" style={{ marginTop: ".6rem" }}>
          <Pagination
            page={page}
            pageCount={Math.max(1, Math.ceil(data.pagination.total / LIST_PAGE_SIZE))}
            onPage={setPage}
          />
        </div>
      ) : null}

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
