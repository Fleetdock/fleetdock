"use client";

import { useEffect, useMemo, useState, type FormEvent } from "react";

import { Pencil, Plus, Trash2 } from "lucide-react";

import { DataTable, type DataTableColumn } from "@/components/data-table";
import { ErrorText, Field, Modal, StatusBadge } from "@/components/ui";
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
import type { Schedule } from "@/lib/types";

const CRON_PRESETS: { label: string; value: string }[] = [
  { label: "Every hour", value: "0 * * * *" },
  { label: "Daily at 02:00", value: "0 2 * * *" },
  { label: "Daily at 14:00", value: "0 14 * * *" },
  { label: "Weekly (Sun 03:00)", value: "0 3 * * 0" },
  { label: "Monthly (1st 04:00)", value: "0 4 1 * *" },
];

function cronLabel(value: string): string {
  return CRON_PRESETS.find((p) => p.value === value)?.label ?? value;
}

export default function SchedulesPage() {
  const { data, isLoading, error } = useSchedules();
  const { data: databases } = useDatabases();
  const { data: destinations } = useDestinations();
  const del = useDeleteSchedule();
  const can = useCan();
  const canWrite = can("schedule:write");

  const [addOpen, setAddOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Schedule | null>(null);

  const dbName = useMemo(() => {
    const m = new Map<string, string>();
    databases?.items.forEach((d) => m.set(d.id, d.name));
    return m;
  }, [databases]);
  const destName = useMemo(() => {
    const m = new Map<string, string>();
    destinations?.items.forEach((d) => m.set(d.id, d.name));
    return m;
  }, [destinations]);

  const table = useDataTable({ items: data?.items });

  const columns = useMemo(() => {
    const cols: DataTableColumn<Schedule>[] = [
      { id: "database", header: "Database", className: "font-medium", render: (s) => dbName.get(s.database_id) ?? "—" },
      { id: "cron", header: "Schedule", className: "muted", render: (s) => cronLabel(s.cron) },
      { id: "destination", header: "Destination", className: "muted", render: (s) => destName.get(s.destination_id) ?? "—" },
      { id: "retention", header: "Retention", className: "muted", render: (s) => `${s.retention_days}d` },
      { id: "enabled", header: "Status", render: (s) => <StatusBadge status={s.enabled ? "active" : "stopped"} /> },
      {
        id: "next",
        header: "Next run",
        className: "muted",
        render: (s) => (s.enabled && s.next_run_at ? new Date(s.next_run_at).toLocaleString() : "—"),
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
              onClick={() => confirm("Delete this schedule? Existing backups are kept.") && del.mutate(s.id)}
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
  }, [canWrite, dbName, destName, del]);

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">Backup schedules</h1>
          <p className="muted text-sm">Recurring backups run by the control plane on a cron schedule, with retention.</p>
        </div>
        {canWrite ? (
          <button className="btn btn-primary" onClick={() => setAddOpen(true)}>
            <Plus size={16} /> New schedule
          </button>
        ) : null}
      </div>

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
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    if (isEdit && schedule) {
      setDatabaseId(schedule.database_id);
      setDestinationId(schedule.destination_id);
      setCron(schedule.cron);
      setRetention(String(schedule.retention_days));
      setEnabled(schedule.enabled);
    } else if (!isEdit) {
      setDatabaseId("");
      setDestinationId("");
      setCron("0 2 * * *");
      setRetention("30");
      setEnabled(true);
    }
    setError(null);
  }, [open, isEdit, schedule]);

  const pending = create.isPending || update.isPending;
  const presetMatch = CRON_PRESETS.some((p) => p.value === cron);

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
        <Field label="Destination">
          <select className="input" value={destinationId} onChange={(e) => setDestinationId(e.target.value)} required>
            <option value="">Select a destination…</option>
            {destinations?.items.map((d) => (
              <option key={d.id} value={d.id}>{d.name}</option>
            ))}
          </select>
        </Field>
        <Field label="Frequency">
          <select className="input" value={presetMatch ? cron : "custom"} onChange={(e) => e.target.value !== "custom" && setCron(e.target.value)}>
            {CRON_PRESETS.map((p) => (
              <option key={p.value} value={p.value}>{p.label}</option>
            ))}
            <option value="custom">Custom (cron)…</option>
          </select>
        </Field>
        {!presetMatch ? (
          <Field label="Cron expression">
            <input className="input" value={cron} onChange={(e) => setCron(e.target.value)} placeholder="0 2 * * *" required />
          </Field>
        ) : null}
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Retention (days)">
            <input className="input" type="number" min={1} value={retention} onChange={(e) => setRetention(e.target.value)} required />
          </Field>
          <Field label="Enabled">
            <select className="input" value={enabled ? "yes" : "no"} onChange={(e) => setEnabled(e.target.value === "yes")}>
              <option value="yes">Enabled</option>
              <option value="no">Paused</option>
            </select>
          </Field>
        </div>
        <p className="muted text-sm">Cron format: minute hour day-of-month month day-of-week (UTC). Backups older than the retention window are deleted automatically.</p>
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
