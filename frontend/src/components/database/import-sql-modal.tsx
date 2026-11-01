"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { Button, ButtonLink, Callout, ErrorText, Field, Input, Modal, Select } from "@/components/ui";
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
          <div className="modal-actions">
            <ButtonLink href={`/activity/${operationId}`}>View operation</ButtonLink>
            <Button variant="primary" onClick={onClose}>
              Done
            </Button>
          </div>
        </>
      ) : (
        <form onSubmit={onSubmit}>
          <p className="text-sm muted mt-0">
            Runs a SQL dump (.sql or .sql.gz, max 2 GB) against this database
            with the same rights as a user with write access in the console —
            statements that need instance-admin rights (creating users, other
            databases) fail. On PostgreSQL the file is applied in one
            transaction.
          </p>
          <Field label="SQL file">
            <Input
              type="file"
              accept=".sql,.gz,application/sql,application/gzip"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              required
            />
          </Field>
          {destinations && destinations.items.length === 0 ? (
            <Callout tone="warning">
              A backup destination is needed to stage the file.{" "}
              <Link href="/backups/storage" className="link">
                Add one
              </Link>{" "}
              first.
            </Callout>
          ) : (
            <Field label="Upload through backup storage">
              <Select
                value={dest}
                onChange={(e) => setDestination(e.target.value)}
              >
                {destinations?.items.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name}
                  </option>
                ))}
              </Select>
            </Field>
          )}
          <ErrorText message={error ?? undefined} />
          <div className="modal-actions">
            <Button onClick={onClose} disabled={busy}>
              Cancel
            </Button>
            <Button
              type="submit"
              variant="primary"
              loading={busy}
              disabled={!file || !dest}
            >
              {busy ? "Uploading…" : "Import"}
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
}
