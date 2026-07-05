"use client";

import Link from "next/link";
import type { ReactNode } from "react";

import { Activity, Archive, Bell, Box, CalendarClock, Database, Server } from "lucide-react";

import { EmptyState, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useOverview } from "@/lib/hooks";
import type { Overview } from "@/lib/types";

export default function DashboardPage() {
  const { data, isLoading, error } = useOverview();

  if (isLoading) {
    return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>;
  }
  if (error || !data) {
    return <EmptyState title="Could not load the dashboard" hint={error ? (error as ApiError).message : undefined} />;
  }

  return (
    <div>
      <div style={{ marginBottom: "1.1rem" }}>
        <h1 className="text-xl font-semibold">Overview</h1>
        <p className="muted text-sm">Fleet health and recent activity across the control plane.</p>
      </div>

      <div className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: ".9rem" }}>
        <StatCard
          href="/servers"
          Icon={Server}
          label="Servers"
          value={data.servers.total}
          sub={
            <span className="flex items-center gap-2">
              <StatusBadge status="online" /> {data.servers.online}
              {data.servers.offline > 0 ? <><StatusBadge status="offline" /> {data.servers.offline}</> : null}
            </span>
          }
        />
        <StatCard
          href="/instances"
          Icon={Box}
          label="Instances"
          value={data.instances.total}
          sub={<span className="muted text-sm">{data.instances.managed} managed · {data.instances.external} external</span>}
        />
        <StatCard
          href="/databases"
          Icon={Database}
          label="Databases"
          value={data.databases.total}
          sub={<span className="muted text-sm">{data.databases.active} active</span>}
        />
        <StatCard
          href="/operations"
          Icon={Activity}
          label="Operations running"
          value={data.operations.running}
          sub={
            data.operations.failed_24h > 0
              ? <span style={{ color: "var(--danger)" }} className="text-sm">{data.operations.failed_24h} failed (24h)</span>
              : <span className="muted text-sm">no failures (24h)</span>
          }
        />
      </div>

      <h2 className="font-semibold" style={{ margin: "1.6rem 0 .7rem" }}>Backups &amp; automation</h2>
      <div className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: ".9rem" }}>
        <StatCard
          href="/backups"
          Icon={Archive}
          label="Backups (24h)"
          value={data.backups.completed_24h}
          sub={
            <span className="text-sm">
              {data.backups.failed_24h > 0 ? (
                <span style={{ color: "var(--danger)" }}>{data.backups.failed_24h} failed · </span>
              ) : null}
              <span className="muted">
                last {data.backups.last_backup_at ? new Date(data.backups.last_backup_at).toLocaleString() : "never"}
              </span>
            </span>
          }
        />
        <StatCard
          href="/schedules"
          Icon={CalendarClock}
          label="Schedules enabled"
          value={data.automation.schedules_enabled}
          sub={<span className="muted text-sm">recurring backups</span>}
        />
        <StatCard
          href="/notifications"
          Icon={Bell}
          label="Alert rules"
          value={data.automation.rules_enabled}
          sub={<span className="muted text-sm">{data.automation.channels_enabled} channels</span>}
        />
      </div>
    </div>
  );
}

function StatCard({
  href,
  Icon,
  label,
  value,
  sub,
}: {
  href: string;
  Icon: typeof Server;
  label: string;
  value: number;
  sub?: ReactNode;
}) {
  return (
    <Link href={href} className="card" style={{ padding: "1.1rem", display: "block", textDecoration: "none", color: "inherit" }}>
      <div className="flex items-center justify-between" style={{ marginBottom: ".6rem" }}>
        <span className="muted text-sm font-medium">{label}</span>
        <Icon size={17} color="var(--muted)" />
      </div>
      <div className="text-2xl font-semibold" style={{ fontSize: "1.75rem", lineHeight: 1 }}>{value}</div>
      {sub ? <div style={{ marginTop: ".55rem" }}>{sub}</div> : null}
    </Link>
  );
}
