"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { ErrorText, Field, Modal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useDestinations, useTriggerBackup } from "@/lib/hooks";
import type { Database } from "@/lib/types";

export function BackupDatabaseModal({
  database,
  onClose,
}: {
  database: Database | null;
  onClose: () => void;
}) {
  const trigger = useTriggerBackup();
  const { data: destinations } = useDestinations();
  const [destinationId, setDestinationId] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await trigger.mutateAsync({
        database_id: database!.id,
        destination_id: destinationId,
      });
      setDone(true);
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : "Failed to start backup",
      );
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
          <p className="text-sm">
            Backup started. It shows up under Backups → History when it
            finishes.
          </p>
          <div
            className="flex justify-end items-center"
            style={{ marginTop: ".8rem" }}
          >
            <button className="btn btn-primary" onClick={close}>
              Done
            </button>
          </div>
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          <Field label="Save to">
            <select
              className="input"
              value={destinationId}
              onChange={(e) => setDestinationId(e.target.value)}
              required
            >
              <option value="" disabled>
                Choose where to store it…
              </option>
              {destinations?.items.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name} ({d.bucket})
                </option>
              ))}
            </select>
          </Field>
          {!destinations || destinations.items.length === 0 ? (
            <p className="text-sm muted">
              You haven&apos;t added backup storage yet.{" "}
              <Link href="/backups/storage" className="link">
                Add a bucket
              </Link>{" "}
              first.
            </p>
          ) : null}
          <ErrorText message={error ?? undefined} />
          <div
            className="flex justify-end items-center gap-2"
            style={{ marginTop: ".5rem" }}
          >
            <button type="button" className="btn" onClick={close}>
              Cancel
            </button>
            <button
              type="submit"
              className="btn btn-primary"
              disabled={
                trigger.isPending ||
                !destinations ||
                destinations.items.length === 0
              }
            >
              {trigger.isPending ? "Starting…" : "Start backup"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
