"use client";

import { useState } from "react";

import { Archive, CalendarClock } from "lucide-react";

import { BackupList } from "@/components/backup/backup-list";
import { BackupDatabaseModal } from "@/components/database/backup-database-modal";
import { Button, ButtonLink, Time } from "@/components/ui";
import { useCan, useSchedules } from "@/lib/hooks";
import type { Database } from "@/lib/types";

/** DatabaseBackups is the Backups tab of a database: its schedule and history. */
export function DatabaseBackups({ database, canBackup }: { database: Database; canBackup: boolean }) {
  const can = useCan();
  const [backupOpen, setBackupOpen] = useState(false);
  const { data: schedules } = useSchedules(database.id);
  const active = (schedules?.items ?? []).filter((s) => s.enabled);
  const next = active
    .map((s) => s.next_run_at)
    .filter((t): t is string => Boolean(t))
    .sort()[0];

  return (
    <div>
      <div className="card flex flex-wrap items-center gap-3 px-4 py-3 mb-3.5">
        <CalendarClock size={18} className="muted" aria-hidden />
        <div className="flex-1 min-w-48">
          {active.length > 0 ? (
            <>
              <div className="font-medium">Backed up automatically</div>
              <div className="muted text-sm">
                {next ? (
                  <>
                    Next backup <Time value={next} />
                  </>
                ) : (
                  "On a schedule"
                )}
              </div>
            </>
          ) : (
            <>
              <div className="font-medium">No automatic backups</div>
              <div className="muted text-sm">Set up a schedule so this database is backed up without you having to remember.</div>
            </>
          )}
        </div>
        {can("schedule:read") ? (
          <ButtonLink href="/backups/schedules" size="sm">
            {active.length > 0 ? "Manage schedule" : "Set up a schedule"}
          </ButtonLink>
        ) : null}
        {canBackup ? (
          <Button
            size="sm"
            variant="primary"
            icon={<Archive size={15} aria-hidden />}
            onClick={() => setBackupOpen(true)}
          >
            Back up now
          </Button>
        ) : null}
      </div>
      <BackupList
        databaseId={database.id}
        emptyHint="Back up this database now, or set up a schedule."
        emptyAction={
          canBackup ? (
            <Button
              variant="primary"
              icon={<Archive size={15} aria-hidden />}
              onClick={() => setBackupOpen(true)}
            >
              Back up now
            </Button>
          ) : undefined
        }
      />
      <BackupDatabaseModal database={backupOpen ? database : null} onClose={() => setBackupOpen(false)} />
    </div>
  );
}
