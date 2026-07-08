"use client";

import { useState } from "react";

import { DataTable } from "@/components/data-table";
import { StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { LIST_PAGE_SIZE, useOperations } from "@/lib/hooks";
import type { Operation } from "@/lib/types";

const TYPE_LABEL: Record<string, string> = {
  create_database: "Create database",
  delete_database: "Delete database",
  backup: "Backup",
  restore: "Restore",
  test_connection: "Test connection",
  import_databases: "Import databases",
  provision_instance: "Provision instance",
  start_instance: "Start instance",
  stop_instance: "Stop instance",
  restart_instance: "Restart instance",
  remove_instance: "Remove instance",
};

export default function OperationsPage() {
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(1);
  const { data, isLoading, error } = useOperations({ status: status || undefined, page });

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">Operations</h1>
          <p className="muted text-sm">Async actions executed by agents and the control plane.</p>
        </div>
      </div>

      <DataTable<Operation>
        columns={[
          { id: "type", header: "Operation", className: "font-medium", render: (op) => TYPE_LABEL[op.type] ?? op.type },
          { id: "executor", header: "Executor", className: "muted", render: (op) => (op.server_id ? "agent" : "control plane") },
          { id: "status", header: "Status", render: (op) => <StatusBadge status={op.status} /> },
          {
            id: "error",
            header: "Error",
            className: "muted",
            render: (op) => (
              <span style={{ maxWidth: 320, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", display: "block" }} title={op.error ?? undefined}>
                {op.error ?? "—"}
              </span>
            ),
          },
          { id: "created", header: "Created", className: "muted", render: (op) => new Date(op.created_at).toLocaleString() },
          {
            id: "finished",
            header: "Finished",
            className: "muted",
            render: (op) => (op.completed_at ? new Date(op.completed_at).toLocaleString() : "—"),
          },
        ]}
        rows={data?.items ?? []}
        rowKey={(op) => op.id}
        isLoading={isLoading}
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not load operations"
        emptyTitle="No operations yet"
        emptyHint="Actions like backups and database creation appear here."
        toolbar={
          <select
            className="input"
            style={{ width: "11rem" }}
            value={status}
            onChange={(e) => {
              setStatus(e.target.value);
              setPage(1);
            }}
          >
            <option value="">All statuses</option>
            <option value="pending">Pending</option>
            <option value="running">Running</option>
            <option value="succeeded">Succeeded</option>
            <option value="failed">Failed</option>
          </select>
        }
        pagination={{
          page,
          pageCount: Math.max(1, Math.ceil((data?.pagination.total ?? 0) / LIST_PAGE_SIZE)),
          onPage: setPage,
        }}
      />
    </div>
  );
}
