"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { Plus, Search } from "lucide-react";

import { BackupList } from "@/components/backup/backup-list";
import { ErrorText, Field, Modal, PageHeader } from "@/components/ui";
import { friendlyError } from "@/lib/errors";
import { useCan, useDatabases, useDestinations, useTriggerBackup } from "@/lib/hooks";

export default function BackupsPage() {
  const can = useCan();
  const canWrite = can("backup:write");
  const { data: databases } = useDatabases({ enabled: canWrite });
  const { data: destinations } = useDestinations(can("destination:read"));
  const [createOpen, setCreateOpen] = useState(false);
  const [search, setSearch] = useState("");
  const noStorage = destinations !== undefined && destinations.items.length === 0;

  return (
    <div>
      <PageHeader
        title="Backup history"
        description="Every backup taken of your databases. Restore one, download it, or check that it restores."
        actions={
          canWrite ? (
            <button className="btn btn-primary" onClick={() => setCreateOpen(true)}>
              <Plus size={16} /> Back up now
            </button>
          ) : null
        }
      />
      <div className="search-box" style={{ marginBottom: ".9rem" }}>
        <Search size={16} aria-hidden />
        <input
          className="input"
          type="search"
          placeholder="Search by database or server…"
          aria-label="Search backups"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>
      <BackupList
        search={search}
        emptyHint={
          noStorage
            ? "Add backup storage first, then back up a database or set up a schedule."
            : "Back up a database now, or set up a schedule to do it automatically."
        }
        emptyAction={
          noStorage ? (
            <Link href="/backups/storage" className="btn btn-primary">
              Add backup storage
            </Link>
          ) : (
            <Link href="/backups/schedules" className="btn">
              Set up a schedule
            </Link>
          )
        }
      />
      <NewBackupModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        databases={databases?.items ?? []}
        destinations={destinations?.items ?? []}
      />
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
      await trigger.mutateAsync({
        database_id: databaseId,
        destination_id: destinationId,
      });
      onClose();
    } catch (err) {
      setError(friendlyError(err, "Failed to start the backup"));
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Back up a database">
      <form onSubmit={onSubmit}>
        <Field label="Database">
          <select
            className="input"
            value={databaseId}
            onChange={(e) => setDatabaseId(e.target.value)}
            required
          >
            <option value="" disabled>
              Select a database…
            </option>
            {databases.map((d) => (
              <option key={d.id} value={d.id}>
                {d.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Save to">
          <select
            className="input"
            value={destinationId}
            onChange={(e) => setDestinationId(e.target.value)}
            required
          >
            <option value="" disabled>
              Select backup storage…
            </option>
            {destinations.map((d) => (
              <option key={d.id} value={d.id}>
                {d.name}
              </option>
            ))}
          </select>
        </Field>
        {destinations.length === 0 ? (
          <p className="muted text-sm">
            Add <Link href="/backups/storage" className="link">backup storage</Link> first.
          </p>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div
          className="flex items-center justify-end gap-2"
          style={{ marginTop: ".5rem" }}
        >
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={trigger.isPending || destinations.length === 0}
          >
            {trigger.isPending ? "Starting…" : "Start backup"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
