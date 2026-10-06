"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { ErrorText, Field, Modal } from "@/components/ui";
import { api, ApiError } from "@/lib/api";
import { useDestinations } from "@/lib/hooks";

/**
 * ImportSQLModal uploads a .sql or .sql.gz file and applies it to the
 * database in the background (as the database's read-write role).
 */
export function ImportSQLModal({
  databaseId,
  databaseName,
  onClose,
}: {
  databaseId: string;
  databaseName: string;
  onClose: () => void;
}) {
  const { data: destinations } = useDestinations();
  const [file, setFile] = useState<File | null>(null);
  const [destination, setDestination] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [operationId, setOperationId] = useState<string | null>(null);

  const dest = destination || destinations?.items[0]?.id || "";

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!file || !dest) return;
    setBusy(true);
    setError(null);
    try {
      const gz = /\.gz$/i.test(file.name);
      const res = await api.upload<{ operation_id: string }>(
        `/v1/databases/${databaseId}/import-sql?destination_id=${dest}${gz ? "&gzip=true" : ""}`,
        file,
        "application/octet-stream",
      );
      setOperationId(res.operation_id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Upload failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal open onClose={onClose} title={`Import SQL into ${databaseName}`}>
      {operationId ? (
        <>
          <p>The file was uploaded and is being applied in the background.</p>
          <div className="flex justify-end gap-2">
            <Link href={`/activity/${operationId}`} className="btn btn-sm">
              View operation
            </Link>
            <button className="btn btn-primary btn-sm" onClick={onClose}>
              Done
            </button>
          </div>
        </>
      ) : (
        <form onSubmit={onSubmit}>
          <p className="text-sm muted" style={{ marginTop: 0 }}>
            Runs a SQL dump (.sql or .sql.gz, max 2 GB) against this database
            with the same rights as a user with write access in the console —
            statements that need instance-admin rights (creating users, other
            databases) fail. On PostgreSQL the file is applied in one
            transaction.
          </p>
          <Field label="SQL file">
            <input
              className="input"
              type="file"
              accept=".sql,.gz,application/sql,application/gzip"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              required
            />
          </Field>
          {destinations && destinations.items.length === 0 ? (
            <p className="text-sm" style={{ color: "var(--danger)" }}>
              A backup destination is needed to stage the file.{" "}
              <Link href="/backups/storage">Add one</Link> first.
            </p>
          ) : (
            <Field label="Upload through backup storage">
              <select
                className="input"
                value={dest}
                onChange={(e) => setDestination(e.target.value)}
              >
                {destinations?.items.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name}
                  </option>
                ))}
              </select>
            </Field>
          )}
          <ErrorText message={error ?? undefined} />
          <div className="flex gap-2" style={{ marginTop: ".8rem" }}>
            <button
              className="btn btn-primary"
              type="submit"
              disabled={busy || !file || !dest}
            >
              {busy ? "Uploading…" : "Import"}
            </button>
            <button
              className="btn"
              type="button"
              onClick={onClose}
              disabled={busy}
            >
              Cancel
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
