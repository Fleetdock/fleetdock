"use client";

import { useState } from "react";

import { EmptyState, Pagination, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { LIST_PAGE_SIZE, useOperations } from "@/lib/hooks";

const TYPE_LABEL: Record<string, string> = {
  create_database: "Create database",
  delete_database: "Delete database",
  backup: "Backup",
  restore: "Restore",
  test_connection: "Test connection",
  import_databases: "Import databases",
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
      </div>

      {isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>
      ) : error ? (
        <EmptyState title="Could not load operations" hint={(error as ApiError).message} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState title="No operations yet" hint="Actions like backups and database creation appear here." />
      ) : (
        <div className="card" style={{ overflow: "hidden" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Operation</th>
                <th>Executor</th>
                <th>Status</th>
                <th>Error</th>
                <th>Created</th>
                <th>Finished</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((op) => (
                <tr key={op.id}>
                  <td className="font-medium">{TYPE_LABEL[op.type] ?? op.type}</td>
                  <td className="muted">{op.server_id ? "agent" : "control plane"}</td>
                  <td><StatusBadge status={op.status} /></td>
                  <td className="muted" style={{ maxWidth: 320, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={op.error ?? undefined}>
                    {op.error ?? "—"}
                  </td>
                  <td className="muted">{new Date(op.created_at).toLocaleString()}</td>
                  <td className="muted">{op.completed_at ? new Date(op.completed_at).toLocaleString() : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {data ? (
        <div className="flex items-center justify-end" style={{ marginTop: ".6rem" }}>
          <Pagination
            page={page}
            pageCount={Math.max(1, Math.ceil(data.pagination.total / LIST_PAGE_SIZE))}
            onPage={setPage}
          />
        </div>
      ) : null}
    </div>
  );
}
