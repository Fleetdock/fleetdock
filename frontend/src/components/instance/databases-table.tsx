"use client";

import Link from "next/link";
import type { ReactNode } from "react";

import { DataTable } from "@/components/data-table";
import { Badge, StatusBadge } from "@/components/ui";
import { formatBytes } from "@/lib/format";
import { LIST_PAGE_SIZE } from "@/lib/hooks";
import type { Database, Paginated } from "@/lib/types";

/** InstanceDatabases is the paginated list of databases on one database server. */
export function InstanceDatabases({
  databases,
  isLoading,
  page,
  onPage,
  hasCredentials,
  emptyAction,
}: {
  databases: Paginated<Database> | undefined;
  isLoading: boolean;
  page: number;
  onPage: (page: number) => void;
  hasCredentials: boolean;
  /** The next step when the list is empty (e.g. "Add login"). */
  emptyAction?: ReactNode;
}) {
  return (
    <DataTable<Database>
      columns={[
        {
          id: "name",
          header: "Name",
          className: "font-medium",
          render: (d) => (
            <span className="flex items-center gap-2">
              <Link href={`/databases/${d.id}`} className="link-plain">
                {d.name}
              </Link>
              {d.system ? <Badge>system</Badge> : null}
            </span>
          ),
        },
        { id: "status", header: "Status", render: (d) => <StatusBadge status={d.status} /> },
        { id: "size", header: "Size", align: "right", className: "muted num", render: (d) => formatBytes(d.size_bytes) },
        {
          id: "conns",
          header: "Connections",
          align: "right",
          className: "muted num",
          hideOnMobile: true,
          render: (d) => d.active_connections ?? 0,
        },
        { id: "charset", header: "Charset", className: "muted", hideOnMobile: true, render: (d) => d.charset },
      ]}
      rows={databases?.items ?? []}
      rowKey={(d) => d.id}
      isLoading={isLoading && !databases}
      emptyTitle="No databases found yet"
      emptyHint={
        hasCredentials
          ? "Databases on this server appear here automatically within a minute."
          : "Add an admin login so Fleetdock can see this server's databases."
      }
      emptyAction={emptyAction}
      pagination={{
        page,
        pageCount: Math.max(1, Math.ceil((databases?.pagination.total ?? 0) / LIST_PAGE_SIZE)),
        onPage,
      }}
    />
  );
}
