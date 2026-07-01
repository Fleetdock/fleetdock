"use client";

import { useMemo, useState, type FormEvent } from "react";

import { LockIcon, PlusIcon, SearchIcon, TrashIcon, UnlockIcon } from "@/components/icons";
import { EmptyState, ErrorText, Field, Modal, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useCreateDatabase,
  useDatabases,
  useDeleteDatabase,
  useInstances,
  useLockDatabase,
  useUnlockDatabase,
} from "@/lib/hooks";

export default function DatabasesPage() {
  const [search, setSearch] = useState("");
  const { data, isLoading, error } = useDatabases({ search });
  const { data: instances } = useInstances();
  const [open, setOpen] = useState(false);

  const lock = useLockDatabase();
  const unlock = useUnlockDatabase();
  const del = useDeleteDatabase();

  const instanceName = useMemo(() => {
    const m = new Map<string, string>();
    instances?.items.forEach((i) => m.set(i.id, i.name));
    return m;
  }, [instances]);

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">Databases</h1>
          <p className="muted text-sm">Logical databases across your instances.</p>
        </div>
        <button className="btn btn-primary" onClick={() => setOpen(true)}>
          <PlusIcon size={16} /> Create database
        </button>
      </div>

      <div className="flex items-center gap-2" style={{ marginBottom: ".9rem", maxWidth: "22rem" }}>
        <div style={{ position: "relative", width: "100%" }}>
          <span style={{ position: "absolute", left: 10, top: 9, color: "var(--muted)" }}>
            <SearchIcon size={16} />
          </span>
          <input
            className="input"
            style={{ paddingLeft: "2rem" }}
            placeholder="Search databases…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
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
                <th style={{ textAlign: "right" }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((d) => (
                <tr key={d.id}>
                  <td className="font-medium">{d.name}</td>
                  <td className="muted">{instanceName.get(d.instance_id) ?? d.instance_id.slice(0, 8)}</td>
                  <td className="muted">{d.charset}</td>
                  <td><StatusBadge status={d.status} /></td>
                  <td style={{ textAlign: "right" }}>
                    <div className="flex items-center gap-2" style={{ justifyContent: "flex-end" }}>
                      {d.status === "locked" ? (
                        <button className="btn btn-sm" onClick={() => unlock.mutate(d.id)} disabled={unlock.isPending}>
                          <UnlockIcon size={15} /> Unlock
                        </button>
                      ) : (
                        <button className="btn btn-sm" onClick={() => lock.mutate(d.id)} disabled={lock.isPending}>
                          <LockIcon size={15} /> Lock
                        </button>
                      )}
                      <button
                        className="btn btn-sm btn-danger"
                        onClick={() => {
                          if (confirm(`Delete database "${d.name}"? It enters a 7-day recovery window.`)) {
                            del.mutate(d.id);
                          }
                        }}
                        disabled={del.isPending}
                        aria-label="Delete"
                      >
                        <TrashIcon size={15} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <CreateDatabaseModal open={open} onClose={() => setOpen(false)} instances={instances?.items ?? []} />
    </div>
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
