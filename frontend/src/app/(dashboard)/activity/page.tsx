"use client";

import { ChevronRight } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { DataTable } from "@/components/data-table";
import { ButtonLink, PageHeader, Select, StatusBadge, Time } from "@/components/ui";
import { LIST_PAGE_SIZE, useOperations } from "@/lib/hooks";
import { operationLabel, resourceHref } from "@/lib/operations";
import { friendlyError } from "@/lib/errors";
import type { Operation } from "@/lib/types";

export default function OperationsPage() {
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(1);
  const { data, isLoading, error, refetch } = useOperations({ status: status || undefined, page });

  return (
    <div>
      <PageHeader
        title="Activity"
        description="Everything Fleetdock has done or is doing: backups, restores, new databases and more. Open one to see its log."
      />

      <DataTable<Operation>
        columns={[
          {
            id: "type",
            header: "Task",
            className: "font-medium",
            render: (op) => (
              <Link href={`/activity/${op.id}`} className="link-plain">
                {operationLabel(op.type)}
              </Link>
            ),
          },
          {
            id: "resource",
            header: "On",
            render: (op) => {
              const href = resourceHref(op.resource_type, op.resource_id);
              const name = op.resource_name || "—";
              return href && op.resource_name ? (
                <Link href={href} className="link-plain">
                  {name}
                </Link>
              ) : (
                <span className="muted">{name}</span>
              );
            },
          },
          { id: "status", header: "Status", render: (op) => <StatusBadge status={op.status} /> },
          {
            id: "error",
            header: "Problem",
            className: "muted",
            hideOnMobile: true,
            render: (op) => (
              <span className="truncate block max-w-80" title={op.error ?? undefined}>
                {op.error ?? ""}
              </span>
            ),
          },
          {
            id: "runner",
            header: "Run by",
            className: "muted",
            hideOnMobile: true,
            render: (op) => (op.server_id ? "Agent" : "Fleetdock"),
          },
          { id: "created", header: "Started", className: "muted", render: (op) => <Time value={op.created_at} /> },
          {
            id: "actions",
            header: "",
            align: "right",
            render: (op) => (
              <ButtonLink
                href={`/activity/${op.id}`}
                variant="ghost"
                size="sm"
                className="btn-icon"
                aria-label={`Open ${operationLabel(op.type)}`}
                icon={<ChevronRight size={15} aria-hidden />}
              />
            ),
          },
        ]}
        rows={data?.items ?? []}
        rowKey={(op) => op.id}
        isLoading={isLoading}
        error={error ? friendlyError(error) : undefined}
        errorTitle="Could not load activity"
        onRetry={() => void refetch()}
        emptyTitle={status ? "No tasks with this status" : "Nothing here yet"}
        emptyHint={
          status
            ? "Try another status, or show all tasks."
            : "Backups, new databases and other tasks appear here as soon as they start."
        }
        toolbar={
          <Select
            size="sm"
            className="w-44"
            aria-label="Filter by status"
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
          </Select>
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
