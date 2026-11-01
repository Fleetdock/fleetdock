"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { Archive, HardDrive, Plus, Search } from "lucide-react";

import { BackupList } from "@/components/backup/backup-list";
import {
  Button,
  ButtonLink,
  ErrorText,
  Field,
  Modal,
  PageHeader,
  SearchInput,
  Select,
  Toolbar,
} from "@/components/ui";
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
            <Button
              variant="primary"
              icon={<Plus size={16} aria-hidden />}
              onClick={() => setCreateOpen(true)}
            >
              Back up now
            </Button>
          ) : null
        }
      />
      <Toolbar>
        <SearchInput
          icon={<Search size={16} aria-hidden />}
          placeholder="Search by database or server…"
          aria-label="Search backups"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </Toolbar>
      <BackupList
        search={search}
        emptyHint={
          noStorage
            ? "Add backup storage first, then back up a database or set up a schedule."
            : "Back up a database now, or set up a schedule to do it automatically."
        }
        emptyAction={
          noStorage ? (
            <ButtonLink
              href="/backups/storage"
              variant="primary"
              icon={<HardDrive size={15} aria-hidden />}
            >
              Add backup storage
            </ButtonLink>
          ) : (
            <>
              {canWrite ? (
                <Button
                  variant="primary"
                  icon={<Archive size={15} aria-hidden />}
                  onClick={() => setCreateOpen(true)}
                >
                  Back up a database
                </Button>
              ) : null}
              <ButtonLink href="/backups/schedules">Set up a schedule</ButtonLink>
            </>
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
          <Select
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
          </Select>
        </Field>
        <Field label="Save to">
          <Select
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
          </Select>
        </Field>
        {destinations.length === 0 ? (
          <p className="muted text-sm">
            Add <Link href="/backups/storage" className="link">backup storage</Link> first.
          </p>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button
            type="submit"
            variant="primary"
            loading={trigger.isPending}
            disabled={destinations.length === 0}
          >
            {trigger.isPending ? "Starting…" : "Start backup"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
