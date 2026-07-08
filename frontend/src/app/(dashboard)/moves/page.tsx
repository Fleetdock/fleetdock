"use client";

import { useMemo } from "react";

import { ArrowRight } from "lucide-react";

import { DataTable } from "@/components/data-table";
import { StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useDatabases, useInstances, useMoves } from "@/lib/hooks";
import type { Move } from "@/lib/types";

const STATUS_LABEL: Record<string, string> = {
  pending: "pending",
  backing_up: "backing up",
  restoring: "restoring",
  completed: "completed",
  failed: "failed",
};

export default function MovesPage() {
  const { data, isLoading, error } = useMoves();
  const { data: databases } = useDatabases();
  const { data: instances } = useInstances();

  const dbName = useMemo(() => {
    const m = new Map<string, string>();
    databases?.items.forEach((d) => m.set(d.id, d.name));
    return m;
  }, [databases]);
  const instName = useMemo(() => {
    const m = new Map<string, string>();
    instances?.items.forEach((i) => m.set(i.id, i.name));
    return m;
  }, [instances]);

  return (
    <div>
      <div style={{ marginBottom: "1.1rem" }}>
        <h1 className="text-xl font-semibold">Database moves</h1>
        <p className="muted text-sm">Back up → restore → verify → optional cutover, tracked end to end.</p>
      </div>

      <DataTable<Move>
        columns={[
          {
            id: "route",
            header: "Move",
            className: "font-medium",
            render: (m) => (
              <span className="flex items-center gap-2">
                {dbName.get(m.source_database_id) ?? m.source_database_id.slice(0, 8)}
                <ArrowRight size={14} color="var(--muted)" />
                {instName.get(m.target_instance_id) ?? m.target_instance_id.slice(0, 8)} / {m.target_database}
              </span>
            ),
          },
          {
            id: "status",
            header: "Status",
            render: (m) => <StatusBadge status={STATUS_LABEL[m.status] ?? m.status} />,
          },
          { id: "cutover", header: "Cutover", className: "muted", render: (m) => (m.drop_source ? "drop source" : "keep source") },
          { id: "tables", header: "Tables", className: "muted", render: (m) => m.table_count ?? "—" },
          {
            id: "error",
            header: "Error",
            className: "muted",
            render: (m) => (
              <span style={{ maxWidth: 280, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", display: "block" }} title={m.error ?? undefined}>
                {m.error ?? "—"}
              </span>
            ),
          },
          { id: "started", header: "Started", className: "muted", render: (m) => new Date(m.created_at).toLocaleString() },
        ]}
        rows={data?.items ?? []}
        rowKey={(m) => m.id}
        isLoading={isLoading}
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not load moves"
        emptyTitle="No moves yet"
        emptyHint="Start a move from a database's detail page to copy or relocate it to another instance."
      />
    </div>
  );
}
