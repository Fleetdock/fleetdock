"use client";

import { useState } from "react";

import { DataTable } from "@/components/data-table";
import { ApiError } from "@/lib/api";
import { LIST_PAGE_SIZE, useAudit } from "@/lib/hooks";
import type { AuditEntry } from "@/lib/types";

const RESOURCE_TYPES = [
  "servers",
  "instances",
  "databases",
  "backups",
  "backup-schedules",
  "backup-destinations",
  "notification-channels",
  "alert-rules",
  "users",
  "roles",
  "tokens",
];

function actor(e: AuditEntry): string {
  const email = e.metadata?.actor_email;
  if (typeof email === "string" && email) return email;
  return e.actor_type;
}

function statusOf(e: AuditEntry): number | undefined {
  const s = e.metadata?.status;
  return typeof s === "number" ? s : undefined;
}

export default function AuditPage() {
  const [resourceType, setResourceType] = useState("");
  const [page, setPage] = useState(1);
  const { data, isLoading, error } = useAudit({ resource_type: resourceType || undefined, page });

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">Audit log</h1>
          <p className="muted text-sm">Append-only, hash-chained record of every change made through the control plane.</p>
        </div>
      </div>

      <DataTable<AuditEntry>
        columns={[
          { id: "time", header: "Time", className: "muted", render: (e) => new Date(e.created_at).toLocaleString() },
          { id: "actor", header: "Actor", className: "font-medium", render: (e) => actor(e) },
          {
            id: "action",
            header: "Action",
            render: (e) => <code style={{ fontSize: ".8rem" }}>{e.action}</code>,
          },
          {
            id: "resource",
            header: "Resource",
            className: "muted",
            render: (e) => (
              <span title={e.resource_id ?? undefined}>
                {e.resource_type}
                {e.resource_id ? ` · ${e.resource_id.slice(0, 8)}` : ""}
              </span>
            ),
          },
          {
            id: "status",
            header: "Status",
            className: "muted",
            render: (e) => statusOf(e) ?? "—",
          },
        ]}
        rows={data?.items ?? []}
        rowKey={(e) => String(e.id)}
        isLoading={isLoading}
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not load the audit log"
        emptyTitle="No audit entries yet"
        emptyHint="Actions like creating databases, running backups and changing users are recorded here."
        toolbar={
          <select
            className="input"
            style={{ width: "13rem" }}
            value={resourceType}
            onChange={(e) => {
              setResourceType(e.target.value);
              setPage(1);
            }}
          >
            <option value="">All resource types</option>
            {RESOURCE_TYPES.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
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
