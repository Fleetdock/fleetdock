"use client";

import Link from "next/link";
import type { ReactNode } from "react";

import { Activity, Archive, Database, Server } from "lucide-react";

import { AttentionList } from "@/components/overview/attention-list";
import { SetupChecklist } from "@/components/overview/setup-checklist";
import { EmptyState, PageHeader, Skeleton, StatusBadge, Time } from "@/components/ui";
import { friendlyError } from "@/lib/errors";
import { useCanAny, useOperations, useOverview } from "@/lib/hooks";
import { operationLabel } from "@/lib/operations";

export default function DashboardPage() {
  const { data, isLoading, error } = useOverview();
  const canAny = useCanAny();

  const header = <PageHeader title="Overview" description="What needs your attention, and how your fleet is doing." />;

  if (isLoading) {
    return (
      <div>
        {header}
        <div className="grid-cards">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} height="5.5rem" />
          ))}
        </div>
      </div>
    );
  }
  if (error || !data) {
    return (
      <div>
        {header}
        <EmptyState title="Could not load the overview" hint={error ? friendlyError(error) : undefined} />
      </div>
    );
  }

  const critical = data.attention.filter((a) => a.severity === "critical").length;

  return (
    <div>
      {header}
      <SetupChecklist setup={data.setup} />

      <div className="grid-cards" style={{ marginBottom: "1.25rem" }}>
        <StatCard
          href="/servers"
          Icon={Server}
          label="Servers"
          value={data.servers.total}
          sub={
            data.servers.offline > 0 ? (
              <span style={{ color: "var(--danger)" }}>{data.servers.offline} offline</span>
            ) : data.servers.total > 0 ? (
              "all online"
            ) : (
              "none connected"
            )
          }
        />
        <StatCard
          href="/databases"
          Icon={Database}
          label="Databases"
          value={data.databases.total}
          sub={`on ${data.instances.total} database server${data.instances.total === 1 ? "" : "s"}`}
        />
        <StatCard
          href="/backups"
          Icon={Archive}
          label="Backups today"
          value={data.backups.completed_24h}
          sub={
            data.backups.failed_24h > 0 ? (
              <span style={{ color: "var(--danger)" }}>{data.backups.failed_24h} failed</span>
            ) : data.backups.last_backup_at ? (
              <>
                last <Time value={data.backups.last_backup_at} />
              </>
            ) : (
              "no backups yet"
            )
          }
        />
        <StatCard
          href="/activity"
          Icon={Activity}
          label="Running now"
          value={data.operations.running}
          sub={
            data.operations.failed_24h > 0 ? (
              <span style={{ color: "var(--danger)" }}>{data.operations.failed_24h} failed today</span>
            ) : (
              "tasks in progress"
            )
          }
        />
      </div>

      <div className="overview-cols">
        <section aria-labelledby="attention-title">
          <h2 id="attention-title" className="section-title">
            Needs attention
            {data.attention.length > 0 ? (
              <span className={`badge ${critical > 0 ? "badge-red" : "badge-amber"}`} style={{ marginLeft: ".5rem" }}>
                {data.attention.length}
              </span>
            ) : null}
          </h2>
          <AttentionList items={data.attention} />
        </section>
        {canAny("operation:read") ? <RecentActivity /> : null}
      </div>
    </div>
  );
}

function RecentActivity() {
  const { data, isLoading } = useOperations({ page: 1 });
  const ops = (data?.items ?? []).slice(0, 8);
  return (
    <section aria-labelledby="recent-title">
      <h2 id="recent-title" className="section-title flex items-center justify-between">
        Recent activity
        <Link href="/activity" className="link text-sm" style={{ fontWeight: 400 }}>
          View all
        </Link>
      </h2>
      <div className="card" style={{ overflow: "hidden" }}>
        {isLoading ? (
          <div style={{ padding: ".9rem" }}>
            <Skeleton height="1rem" />
          </div>
        ) : ops.length === 0 ? (
          <p className="muted text-sm" style={{ padding: ".9rem", margin: 0 }}>
            Nothing yet. Backups, new databases and other tasks show up here.
          </p>
        ) : (
          <ul className="recent-list">
            {ops.map((op) => (
              <li key={op.id}>
                <Link href={`/activity/${op.id}`}>
                  <span className="truncate" style={{ flex: 1 }}>
                    {operationLabel(op.type)}
                  </span>
                  <StatusBadge status={op.status} />
                  <span className="muted text-sm hide-sm" style={{ minWidth: "5.5rem", textAlign: "right" }}>
                    <Time value={op.created_at} />
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
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
    <Link href={href} className="card stat-card">
      <div className="flex items-center justify-between" style={{ marginBottom: ".5rem" }}>
        <span className="muted text-sm font-medium">{label}</span>
        <Icon size={16} color="var(--muted)" aria-hidden />
      </div>
      <div className="stat-value">{value}</div>
      {sub ? (
        <div className="muted text-sm" style={{ marginTop: ".45rem" }}>
          {sub}
        </div>
      ) : null}
    </Link>
  );
}
