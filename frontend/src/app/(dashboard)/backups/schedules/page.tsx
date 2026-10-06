"use client";

import { useErrorToast } from "@/components/toast";

import Link from "next/link";
import { useEffect, useMemo, useState, type FormEvent } from "react";

import { Pencil, Plus, Trash2 } from "lucide-react";

import { DataTable, type DataTableColumn } from "@/components/data-table";
import { ErrorText, Field, Modal, PageHeader, Time } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useCan,
  useCreateSchedule,
  useDatabases,
  useDeleteSchedule,
  useDestinations,
  useSchedules,
  useUpdateSchedule,
} from "@/lib/hooks";
import { useDataTable } from "@/lib/use-data-table";
import { describeCron, isCron } from "@/lib/cron";
import type { Schedule } from "@/lib/types";
import { useConfirm } from "@/components/confirm";

const CRON_PRESETS: { label: string; value: string }[] = [
  { label: "Every hour", value: "0 * * * *" },
  { label: "Every day at 02:00 UTC", value: "0 2 * * *" },
  { label: "Every day at 14:00 UTC", value: "0 14 * * *" },
  { label: "Every Sunday at 03:00 UTC", value: "0 3 * * 0" },
  { label: "On the 1st of every month at 04:00 UTC", value: "0 4 1 * *" },
];

function cronLabel(value: string): string {
  return describeCron(value) ?? `${value} (UTC)`;
}

export default function SchedulesPage() {
  const { data, isLoading, error } = useSchedules();
  const { data: destinations } = useDestinations();
  const del = useDeleteSchedule();
  const toastError = useErrorToast();
  const confirm = useConfirm();
  const can = useCan();
  const canWrite = can("schedule:write");

  const [addOpen, setAddOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Schedule | null>(null);

  const destName = useMemo(() => {
    const m = new Map<string, string>();
    destinations?.items.forEach((d) => m.set(d.id, d.name));
    return m;
  }, [destinations]);

  const table = useDataTable({ items: data?.items });

  const columns = useMemo(() => {
    const cols: DataTableColumn<Schedule>[] = [
      {
        id: "database",
        header: "Database",
        className: "font-medium",
        render: (s) => (
          <span className="flex flex-col">
            <Link href={`/databases/${s.database_id}`} className="link-plain">
              {s.database_name || "Removed database"}
            </Link>
            {s.instance_name ? <span className="muted text-sm">{s.instance_name}</span> : null}
          </span>
        ),
      },
      { id: "cron", header: "How often", className: "muted", render: (s) => cronLabel(s.cron) },
      { id: "destination", header: "Saved to", className: "muted", hideOnMobile: true, render: (s) => destName.get(s.destination_id) ?? "—" },
      {
        id: "retention",
        header: "Kept for",
        className: "muted",
        hideOnMobile: true,
        render: (s) => `${s.retention_days} day${s.retention_days === 1 ? "" : "s"}`,
      },
      {
        id: "enabled",
        header: "Status",
        render: (s) => (s.enabled ? <span className="badge badge-green">on</span> : <span className="badge badge-gray">paused</span>),
      },
      {
        id: "next",
        header: "Next backup",
        className: "muted",
        render: (s) => (s.enabled && s.next_run_at ? <Time value={s.next_run_at} /> : "—"),
      },
    ];
    if (canWrite) {
      cols.push({
        id: "actions",
        header: "Actions",
        align: "right",
        render: (s) => (
          <div className="flex items-center gap-2" style={{ justifyContent: "flex-end" }}>
            <button className="btn btn-sm" onClick={() => setEditTarget(s)} aria-label="Edit"><Pencil size={15} /></button>
            <button
              className="btn btn-sm btn-danger"
              onClick={async () => {
                const ok = await confirm({
                  title: "Delete this schedule?",
                  message: "Automatic backups of this database stop. Existing backups are kept.",
                  confirmLabel: "Delete schedule",
                  danger: true,
                });
                if (ok) del.mutate(s.id, { onError: toastError("Failed to delete the schedule") });
              }}
              disabled={del.isPending}
              aria-label="Delete"
            >
              <Trash2 size={15} />
            </button>
          </div>
        ),
      });
    }
    return cols;
  }, [toastError, canWrite, confirm, destName, del]);

  return (
    <div>
      <PageHeader
        title="Backup schedules"
        description="Automatic backups on a timetable, and how long to keep them. Times are in UTC."
        actions={
          <>
            {canWrite ? (
          <button className="btn btn-primary" onClick={() => setAddOpen(true)}>
            <Plus size={16} /> New schedule
          </button>
        ) : null}
          </>
        }
      />

      <DataTable<Schedule>
        columns={columns}
        rows={table.rows}
        rowKey={(s) => s.id}
        isLoading={isLoading}
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not load schedules"
        emptyTitle="No schedules yet"
        emptyHint="Create a schedule to back up a database automatically."
        pagination={{ page: table.page, pageCount: table.pageCount, onPage: table.setPage }}
      />

      <ScheduleModal mode="create" open={addOpen} onClose={() => setAddOpen(false)} />
      <ScheduleModal mode="edit" schedule={editTarget} open={editTarget !== null} onClose={() => setEditTarget(null)} />
    </div>
  );
}

type ScheduleModalProps =
  | { mode: "create"; schedule?: undefined; open: boolean; onClose: () => void }
  | { mode: "edit"; schedule: Schedule | null; open: boolean; onClose: () => void };

function ScheduleModal({ mode, schedule, open, onClose }: ScheduleModalProps) {
  const create = useCreateSchedule();
  const update = useUpdateSchedule();
  const { data: databases } = useDatabases();
  const { data: destinations } = useDestinations();
  const isEdit = mode === "edit";

  const [databaseId, setDatabaseId] = useState("");
  const [destinationId, setDestinationId] = useState("");
  const [cron, setCron] = useState("0 2 * * *");
  const [retention, setRetention] = useState("30");
  const [customCron, setCustomCron] = useState(false);
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    if (isEdit && schedule) {
      setDatabaseId(schedule.database_id);
      setDestinationId(schedule.destination_id);
      setCron(schedule.cron);
      setCustomCron(false);
      setRetention(String(schedule.retention_days));
      setEnabled(schedule.enabled);
    } else if (!isEdit) {
      setDatabaseId("");
      setDestinationId("");
      setCron("0 2 * * *");
      setCustomCron(false);
      setRetention("30");
      setEnabled(true);
    }
    setError(null);
  }, [open, isEdit, schedule]);

  const pending = create.isPending || update.isPending;
  const presetMatch = !customCron && CRON_PRESETS.some((p) => p.value === cron);
  const cronText = describeCron(cron);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      if (isEdit && schedule) {
        await update.mutateAsync({
          id: schedule.id,
          destination_id: destinationId,
          cron,
          retention_days: Number(retention),
          enabled,
        });
      } else {
        await create.mutateAsync({
          database_id: databaseId,
          destination_id: destinationId,
          cron,
          retention_days: Number(retention),
          enabled,
        });
      }
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : `Failed to ${isEdit ? "update" : "create"} schedule`);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={isEdit ? "Edit schedule" : "New backup schedule"}>
      <form onSubmit={onSubmit}>
        {!isEdit ? (
          <Field label="Database">
            <select className="input" value={databaseId} onChange={(e) => setDatabaseId(e.target.value)} required>
              <option value="">Select a database…</option>
              {databases?.items.map((d) => (
                <option key={d.id} value={d.id}>{d.name}</option>
              ))}
            </select>
          </Field>
        ) : null}
        <Field label="Save to">
          <select className="input" value={destinationId} onChange={(e) => setDestinationId(e.target.value)} required>
            <option value="">Select backup storage…</option>
            {destinations?.items.map((d) => (
              <option key={d.id} value={d.id}>{d.name}</option>
            ))}
          </select>
        </Field>
        <Field label="How often">
          <select
            className="input"
            value={presetMatch ? cron : "custom"}
            onChange={(e) => {
              const custom = e.target.value === "custom";
              setCustomCron(custom);
              if (!custom) setCron(e.target.value);
            }}
          >
            {CRON_PRESETS.map((p) => (
              <option key={p.value} value={p.value}>{p.label}</option>
            ))}
            <option value="custom">Custom…</option>
          </select>
        </Field>
        {!presetMatch ? (
          <Field
            label="Cron expression"
            hint={isCron(cron) ? (cronText ?? "Custom schedule (UTC)") : "Five fields: minute hour day-of-month month day-of-week, in UTC"}
            help="Example: 0 2 * * * runs every day at 02:00 UTC; */30 * * * * runs every 30 minutes."
          >
            <input className="input mono" value={cron} onChange={(e) => setCron(e.target.value)} placeholder="0 2 * * *" required />
          </Field>
        ) : null}
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Keep backups for (days)" help="Older backups from this schedule are deleted automatically to save storage.">
            <input className="input" type="number" min={1} value={retention} onChange={(e) => setRetention(e.target.value)} required />
          </Field>
          <Field label="Status">
            <select className="input" value={enabled ? "yes" : "no"} onChange={(e) => setEnabled(e.target.value === "yes")}>
              <option value="yes">Enabled</option>
              <option value="no">Paused</option>
            </select>
          </Field>
        </div>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={pending}>
            {pending ? "Saving…" : isEdit ? "Save changes" : "Create schedule"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
