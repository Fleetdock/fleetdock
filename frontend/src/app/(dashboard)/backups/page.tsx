"use client";

import { useMemo, useState, type FormEvent } from "react";

import { Plus, Upload } from "lucide-react";
import { EmptyState, ErrorText, Field, Modal, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useBackups,
  useCan,
  useDatabases,
  useDestinations,
  useInstances,
  useRestoreBackup,
  useTriggerBackup,
} from "@/lib/hooks";
import type { Backup } from "@/lib/types";

function formatBytes(n?: number | null) {
  if (!n) return "—";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let u = 0;
  while (v >= 1024 && u < units.length - 1) {
    v /= 1024;
    u++;
  }
  return `${v.toFixed(u === 0 ? 0 : 1)} ${units[u]}`;
}

export default function BackupsPage() {
  const { data, isLoading, error } = useBackups();
  const { data: databases } = useDatabases();
  const { data: destinations } = useDestinations();
  const [createOpen, setCreateOpen] = useState(false);
  const [restoreTarget, setRestoreTarget] = useState<Backup | null>(null);
  const can = useCan();
  const canWrite = can("backup:write");

  const dbName = useMemo(() => {
    const m = new Map<string, string>();
    databases?.items.forEach((d) => m.set(d.id, d.name));
    return m;
  }, [databases]);

  const destName = useMemo(() => {
    const m = new Map<string, string>();
    destinations?.items.forEach((d) => m.set(d.id, d.name));
    return m;
  }, [destinations]);

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">Backups</h1>
          <p className="muted text-sm">Database dumps stored in S3 / Cloudflare R2.</p>
        </div>
        {canWrite ? (
          <button className="btn btn-primary" onClick={() => setCreateOpen(true)}>
            <Plus size={16} /> New backup
          </button>
        ) : null}
      </div>

      {isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>
      ) : error ? (
        <EmptyState title="Could not load backups" hint={(error as ApiError).message} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          title="No backups yet"
          hint="Add a backup destination, then trigger your first backup."
        />
      ) : (
        <div className="card" style={{ overflow: "hidden" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Database</th>
                <th>Destination</th>
                <th>Status</th>
                <th>Size</th>
                <th>Created</th>
                {canWrite ? <th style={{ textAlign: "right" }}>Actions</th> : null}
              </tr>
            </thead>
            <tbody>
              {data.items.map((b) => (
                <tr key={b.id}>
                  <td className="font-medium">{dbName.get(b.database_id) ?? b.database_id.slice(0, 8)}</td>
                  <td className="muted">{b.destination_id ? destName.get(b.destination_id) ?? "—" : "—"}</td>
                  <td>
                    <StatusBadge status={b.status} />
                    {b.error ? (
                      <div className="muted text-sm" style={{ maxWidth: 280, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={b.error}>
                        {b.error}
                      </div>
                    ) : null}
                  </td>
                  <td className="muted">{formatBytes(b.size_bytes)}</td>
                  <td className="muted">{new Date(b.created_at).toLocaleString()}</td>
                  {canWrite ? (
                    <td style={{ textAlign: "right" }}>
                      {b.status === "completed" ? (
                        <button className="btn btn-sm" onClick={() => setRestoreTarget(b)}>
                          <Upload size={15} /> Restore / Move
                        </button>
                      ) : null}
                    </td>
                  ) : null}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <NewBackupModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        databases={databases?.items ?? []}
        destinations={destinations?.items ?? []}
      />
      <RestoreModal backup={restoreTarget} onClose={() => setRestoreTarget(null)} dbName={dbName} />
    </div>
  );
}

function NewBackupModal({
  open,
  onClose,
  databases,
  destinations,
}: {
  open: boolean;
  onClose: () => void;
  databases: { id: string; name: string }[];
  destinations: { id: string; name: string }[];
}) {
  const trigger = useTriggerBackup();
  const [databaseId, setDatabaseId] = useState("");
  const [destinationId, setDestinationId] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await trigger.mutateAsync({ database_id: databaseId, destination_id: destinationId });
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to start backup");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="New backup">
      <form onSubmit={onSubmit}>
        <Field label="Database">
          <select className="input" value={databaseId} onChange={(e) => setDatabaseId(e.target.value)} required>
            <option value="" disabled>Select a database…</option>
            {databases.map((d) => (
              <option key={d.id} value={d.id}>{d.name}</option>
            ))}
          </select>
        </Field>
        <Field label="Destination">
          <select className="input" value={destinationId} onChange={(e) => setDestinationId(e.target.value)} required>
            <option value="" disabled>Select a destination…</option>
            {destinations.map((d) => (
              <option key={d.id} value={d.id}>{d.name}</option>
            ))}
          </select>
        </Field>
        {destinations.length === 0 ? (
          <p className="muted text-sm">Add an S3/R2 destination first (Destinations page).</p>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={trigger.isPending || destinations.length === 0}>
            {trigger.isPending ? "Starting…" : "Start backup"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function RestoreModal({
  backup,
  onClose,
  dbName,
}: {
  backup: Backup | null;
  onClose: () => void;
  dbName: Map<string, string>;
}) {
  const restore = useRestoreBackup();
  const { data: instances } = useInstances();
  const [targetInstance, setTargetInstance] = useState("");
  const [targetDatabase, setTargetDatabase] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await restore.mutateAsync({
        backup_id: backup!.id,
        target_instance_id: targetInstance || undefined,
        target_database: targetDatabase || undefined,
      });
      setDone(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to start restore");
    }
  }

  function close() {
    setTargetInstance("");
    setTargetDatabase("");
    setDone(false);
    setError(null);
    onClose();
  }

  if (!backup) return null;
  return (
    <Modal open onClose={close} title={`Restore "${dbName.get(backup.database_id) ?? "database"}"`}>
      {done ? (
        <div>
          <p className="text-sm">Restore started — track it on the Operations page.</p>
          <div className="flex items-center justify-end" style={{ marginTop: ".8rem" }}>
            <button className="btn btn-primary" onClick={close}>Done</button>
          </div>
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          <p className="muted text-sm" style={{ marginBottom: ".8rem" }}>
            Restore into the original instance, or pick another instance and/or database name —
            that&apos;s how you move a database to a different server.
          </p>
          <Field label="Target instance (default: original)">
            <select className="input" value={targetInstance} onChange={(e) => setTargetInstance(e.target.value)}>
              <option value="">Original instance</option>
              {instances?.items.filter((i) => i.has_credentials).map((i) => (
                <option key={i.id} value={i.id}>
                  {i.name} ({i.kind === "external" ? `${i.host}:${i.port}` : `port ${i.port}`})
                </option>
              ))}
            </select>
          </Field>
          <Field label="Target database name (default: original)">
            <input className="input" value={targetDatabase} onChange={(e) => setTargetDatabase(e.target.value)} placeholder="leave empty to keep the name" />
          </Field>
          <ErrorText message={error ?? undefined} />
          <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
            <button type="button" className="btn" onClick={close}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={restore.isPending}>
              {restore.isPending ? "Starting…" : "Start restore"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
