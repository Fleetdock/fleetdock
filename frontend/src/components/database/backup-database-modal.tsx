"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import { CloudUpload, Download } from "lucide-react";

import { Button, ErrorText, Field, Modal, Select } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { downloadDatabaseDump, useDestinations, useSchemas, useTriggerBackup } from "@/lib/hooks";
import type { Database } from "@/lib/types";

type Target = "download" | "storage";

export function BackupDatabaseModal({
  database,
  schema: initialSchema,
  onClose,
}: {
  database: Database | null;
  /** PostgreSQL: start with this schema chosen instead of the whole database. */
  schema?: string;
  onClose: () => void;
}) {
  const trigger = useTriggerBackup();
  const pg = database?.instance?.engine === "postgres";
  const { data: schemas } = useSchemas(database?.id ?? "", pg);
  // "" is the whole database.
  const [scope, setScope] = useState(initialSchema ?? "");
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
        await downloadDatabaseDump(database!.id, scope || undefined);
      } else {
        await trigger.mutateAsync({
          database_id: database!.id,
          destination_id: destinationId,
          schema: scope || undefined,
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
    setScope(initialSchema ?? "");
    setPicked(null);
    setDestinationId("");
    setDone(null);
    setError(null);
    onClose();
  }

  if (!database) return null;
  const busy = trigger.isPending || preparing;
  return (
    <Modal open onClose={close} title={scope ? `Back up schema ${scope} of "${database.name}"` : `Back up "${database.name}"`}>
      {done ? (
        <div>
          {done === "download" ? (
            <>
              <p className="text-sm">
                Your download has started: <code>{database.name}{scope ? `-${scope}` : ""}-….sql.gz</code>. A large database can take a
                while — keep this tab open until your browser finishes it.
              </p>
              <p className="text-sm muted mt-2">
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
          <div className="modal-actions">
            <Button variant="primary" onClick={close}>
              Done
            </Button>
          </div>
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          {pg && (schemas?.length ?? 0) > 0 ? (
            <Field
              label="What to back up"
              hint={
                scope
                  ? "Only this schema's objects and rows. Restoring it needs the schemas it refers to in the target database."
                  : undefined
              }
            >
              <Select value={scope} onChange={(e) => setScope(e.target.value)}>
                <option value="">The whole database</option>
                {schemas!.map((s) => (
                  <option key={s.name} value={s.name}>
                    Schema {s.name}
                  </option>
                ))}
              </Select>
            </Field>
          ) : null}
          <div role="radiogroup" aria-label="Where to put the backup" className="choice-list">
            <button
              type="button"
              role="radio"
              aria-checked={target === "download"}
              className="choice-card"
              onClick={() => setPicked("download")}
            >
              <span className="choice-card-icon">
                <Download size={16} aria-hidden />
              </span>
              <span>
                <span className="font-medium">Download to this computer</span>
                <span className="muted text-sm block mt-0.5">
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
                <CloudUpload size={16} aria-hidden />
              </span>
              <span>
                <span className="font-medium">Save to backup storage</span>
                <span className="muted text-sm block mt-0.5">
                  Kept in your bucket, listed in the backup history and restorable from here.
                </span>
              </span>
            </button>
          </div>

          {target === "storage" ? (
            hasStorage ? (
              <Field label="Save to">
                <Select
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
                </Select>
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
          <div className="modal-actions">
            <Button onClick={close}>Cancel</Button>
            <Button
              type="submit"
              variant="primary"
              loading={busy}
              disabled={target === "storage" && !hasStorage}
            >
              {target === "download"
                ? preparing
                  ? "Preparing…"
                  : "Download backup"
                : trigger.isPending
                  ? "Starting…"
                  : "Start backup"}
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
}
