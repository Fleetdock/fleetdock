"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import { CloudUpload, Download } from "lucide-react";

import { ErrorText, Field, Modal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { downloadDatabaseDump, useDestinations, useTriggerBackup } from "@/lib/hooks";
import type { Database } from "@/lib/types";

type Target = "download" | "storage";

export function BackupDatabaseModal({
  database,
  onClose,
}: {
  database: Database | null;
  onClose: () => void;
}) {
  const trigger = useTriggerBackup();
  const { data: destinations } = useDestinations();
  const hasStorage = (destinations?.items.length ?? 0) > 0;
  // Until the user picks, default to storage when there is some and to a
  // download when there is none — a download needs no setup.
  const [picked, setPicked] = useState<Target | null>(null);
  const target: Target = picked ?? (hasStorage ? "storage" : "download");
  const [destinationId, setDestinationId] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState<Target | null>(null);
  const [preparing, setPreparing] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      if (target === "download") {
        setPreparing(true);
        await downloadDatabaseDump(database!.id);
      } else {
        await trigger.mutateAsync({
          database_id: database!.id,
          destination_id: destinationId,
        });
      }
      setDone(target);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : target === "download"
            ? "Failed to start the download"
            : "Failed to start backup",
      );
    } finally {
      setPreparing(false);
    }
  }

  function close() {
    setPicked(null);
    setDestinationId("");
    setDone(null);
    setError(null);
    onClose();
  }

  if (!database) return null;
  const busy = trigger.isPending || preparing;
  return (
    <Modal open onClose={close} title={`Back up "${database.name}"`}>
      {done ? (
        <div>
          {done === "download" ? (
            <>
              <p className="text-sm">
                Your download has started: <code>{database.name}-….sql.gz</code>. A large database can take a
                while — keep this tab open until your browser finishes it.
              </p>
              <p className="text-sm muted" style={{ marginTop: ".5rem" }}>
                The file is not kept in Fleetdock and does not appear in the backup history. To restore it, use
                Import SQL on a database.
              </p>
            </>
          ) : (
            <p className="text-sm">
              Backup started. It shows up under Backups → History when it
              finishes.
            </p>
          )}
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
          <div role="radiogroup" aria-label="Where to put the backup" className="choice-list">
            <button
              type="button"
              role="radio"
              aria-checked={target === "download"}
              className="choice-card"
              onClick={() => setPicked("download")}
            >
              <span className="choice-card-icon">
                <Download size={16} />
              </span>
              <span>
                <span className="font-medium">Download to this computer</span>
                <span className="muted text-sm" style={{ display: "block", marginTop: ".15rem" }}>
                  A compressed SQL dump (.sql.gz), saved by your browser. Nothing is stored in Fleetdock.
                </span>
              </span>
            </button>
            <button
              type="button"
              role="radio"
              aria-checked={target === "storage"}
              className="choice-card"
              onClick={() => setPicked("storage")}
            >
              <span className="choice-card-icon">
                <CloudUpload size={16} />
              </span>
              <span>
                <span className="font-medium">Save to backup storage</span>
                <span className="muted text-sm" style={{ display: "block", marginTop: ".15rem" }}>
                  Kept in your bucket, listed in the backup history and restorable from here.
                </span>
              </span>
            </button>
          </div>

          {target === "storage" ? (
            hasStorage ? (
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
            ) : (
              <p className="text-sm muted">
                You haven&apos;t added backup storage yet.{" "}
                <Link href="/backups/storage" className="link">
                  Add a bucket
                </Link>{" "}
                to keep backups in Fleetdock, or download this one instead.
              </p>
            )
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
              disabled={busy || (target === "storage" && !hasStorage)}
            >
              {target === "download"
                ? preparing
                  ? "Preparing…"
                  : "Download backup"
                : trigger.isPending
                  ? "Starting…"
                  : "Start backup"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
