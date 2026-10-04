"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { ErrorText, Field, Modal } from "@/components/ui";

import { ApiError } from "@/lib/api";
import { useDestinations, useInstances, useStartMove } from "@/lib/hooks";
import type { Database } from "@/lib/types";

export function MoveDatabaseModal({
  database,
  onClose,
}: {
  database: Database | null;
  onClose: () => void;
}) {
  const start = useStartMove();
  const { data: instances } = useInstances();
  const { data: destinations } = useDestinations();
  const [targetInstance, setTargetInstance] = useState("");
  const [targetName, setTargetName] = useState("");
  const [destination, setDestination] = useState("");
  const [dropSource, setDropSource] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [started, setStarted] = useState<string | null>(null);
  const [operationId, setOperationId] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!database) return;
    setError(null);
    try {
      const res = await start.mutateAsync({
        source_database_id: database.id,
        target_instance_id: targetInstance,
        target_database: targetName || undefined,
        destination_id: destination,
        drop_source: dropSource,
      });
      setOperationId(res.operation_id);
      setStarted(
        "Move started — it will back up, then restore and verify in the background. Follow it in Activity.",
      );
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to start move");
    }
  }

  if (!database) return null;
  return (
    <Modal open onClose={onClose} title={`Copy, move or rename "${database.name}"`}>
      {started ? (
        <div>
          <div
            className="card"
            style={{ padding: ".8rem .9rem", marginBottom: "1rem" }}
          >
            <span className="text-sm">{started}</span>
          </div>
          <div className="flex justify-end items-center gap-2">
            <Link
              href={operationId ? `/activity/${operationId}` : "/activity"}
              className="btn btn-sm"
            >
              View operation
            </Link>
            <button className="btn btn-primary btn-sm" onClick={onClose}>
              Done
            </button>
          </div>
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          <Field label="Copy to database server">
            <select
              className="input"
              value={targetInstance}
              onChange={(e) => setTargetInstance(e.target.value)}
              required
            >
              <option value="">Select a database server…</option>
              {(instances?.items ?? [])
                .filter(
                  (i) => i.id !== database.instance_id && i.has_credentials,
                )
                .map((i) => (
                  <option key={i.id} value={i.id}>
                    {i.name} ({i.engine})
                  </option>
                ))}
            </select>
          </Field>
          <Field label="Target database name (optional)">
            <input
              className="input"
              value={targetName}
              onChange={(e) => setTargetName(e.target.value)}
              placeholder={database.name}
            />
          </Field>
          <Field label="Backup storage to use">
            <select
              className="input"
              value={destination}
              onChange={(e) => setDestination(e.target.value)}
              required
            >
              <option value="">Select backup storage…</option>
              {(destinations?.items ?? []).map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name}
                </option>
              ))}
            </select>
          </Field>
          <label
            className="flex items-center gap-2 text-sm"
            style={{ cursor: "pointer", margin: ".2rem 0 .6rem" }}
          >
            <input
              type="checkbox"
              checked={dropSource}
              onChange={(e) => setDropSource(e.target.checked)}
            />
            Drop the source database after a successful, verified move (cutover)
          </label>
          <p className="text-sm muted">
            The source is backed up and restored to the target (verifying the
            checksum and table count), in the background. Leave &quot;drop the
            source&quot; unticked to <strong>copy</strong>; pick the same instance
            with a new name and tick it to <strong>rename</strong>; pick another
            instance to <strong>move</strong>.
          </p>
          <ErrorText message={error ?? undefined} />
          <div
            className="flex justify-end items-center gap-2"
            style={{ marginTop: ".5rem" }}
          >
            <button type="button" className="btn" onClick={onClose}>
              Cancel
            </button>
            <button
              type="submit"
              className="btn btn-primary"
              disabled={start.isPending}
            >
              {start.isPending ? "Starting…" : "Start move"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
