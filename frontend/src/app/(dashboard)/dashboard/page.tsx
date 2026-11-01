"use client";

import Link from "next/link";

import { Activity, Archive, ArrowRight, Database, History, Server } from "lucide-react";

import { AttentionList } from "@/components/overview/attention-list";
import { SetupChecklist } from "@/components/overview/setup-checklist";
import { Badge, Card, CardHeader, PageHeader, PageSkeleton, QueryError, Skeleton, StatTile, StatusBadge, Time } from "@/components/ui";
import { useCanAny, useOperations, useOverview } from "@/lib/hooks";
import { operationLabel } from "@/lib/operations";

export default function DashboardPage() {
  const { data, isLoading, error, refetch } = useOverview();
  const canAny = useCanAny();

  const header = <PageHeader title="Overview" description="What needs your attention, and how your fleet is doing." />;

  if (isLoading) {
    return (
      <PageSkeleton label="Loading overview" stats>
        <div className="overview-cols" aria-hidden>
          <Skeleton height="12rem" radius="var(--radius)" />
          <Skeleton height="12rem" radius="var(--radius)" />
        </div>
      </PageSkeleton>
    );
  }
  if (error || !data) {
    return (
      <div>
        {header}
        <QueryError error={error} what="overview" onRetry={() => void refetch()} />
      </div>
    );
  }

  const critical = data.attention.filter((a) => a.severity === "critical").length;
  const instances = data.instances.total;

  return (
    <div>
      {header}
      <SetupChecklist setup={data.setup} />

      <div className="stat-strip">
        <StatTile
          href="/servers"
          icon={<Server size={14} aria-hidden />}
          label="Servers"
          value={data.servers.total}
          hintTone={data.servers.offline > 0 ? "danger" : undefined}
          hint={data.servers.offline > 0 ? `${data.servers.offline} offline` : data.servers.total > 0 ? "All online" : "None connected"}
        />
        <StatTile
          href="/databases"
          icon={<Database size={14} aria-hidden />}
          label="Databases"
          value={data.databases.total}
          hint={`On ${instances} database server${instances === 1 ? "" : "s"}`}
        />
        <StatTile
          href="/backups"
          icon={<Archive size={14} aria-hidden />}
          label="Backups today"
          value={data.backups.completed_24h}
          hintTone={data.backups.failed_24h > 0 ? "danger" : undefined}
          hint={
            data.backups.failed_24h > 0 ? (
              `${data.backups.failed_24h} failed`
            ) : data.backups.last_backup_at ? (
              <>
                Last <Time value={data.backups.last_backup_at} />
              </>
            ) : (
              "No backups yet"
            )
          }
        />
        <StatTile
          href="/activity"
          icon={<Activity size={14} aria-hidden />}
          label="Running now"
          value={data.operations.running}
          hintTone={data.operations.failed_24h > 0 ? "danger" : undefined}
          hint={data.operations.failed_24h > 0 ? `${data.operations.failed_24h} failed today` : "Tasks in progress"}
        />
      </div>

      <div className="overview-cols">
        <section aria-labelledby="attention-title">
          <h2 id="attention-title" className="section-title flex items-center gap-2">
            Needs attention
            {data.attention.length > 0 ? <Badge tone={critical > 0 ? "danger" : "warning"}>{data.attention.length}</Badge> : null}
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
      <h2 id="recent-title" className="sr-only">
        Recent activity
      </h2>
      <Card className="overflow-hidden">
        <CardHeader
          title="Recent activity"
          icon={<History size={15} aria-hidden className="text-muted" />}
          actions={
            <Link href="/activity" className="link text-sm inline-flex items-center gap-1">
              View all <ArrowRight size={13} aria-hidden />
            </Link>
          }
        />
        {isLoading ? (
          <div className="p-4 stack-sm" aria-hidden>
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} height=".95rem" width={`${85 - i * 12}%`} />
            ))}
          </div>
        ) : ops.length === 0 ? (
          <p className="muted text-sm m-0 p-4">Nothing yet. Backups, new databases and other tasks show up here.</p>
        ) : (
          <ul className="recent-list">
            {ops.map((op) => (
              <li key={op.id}>
                <Link href={`/activity/${op.id}`}>
                  <span className="truncate flex-1">{operationLabel(op.type)}</span>
                  <StatusBadge status={op.status} />
                  <span className="muted text-xs hide-sm min-w-20 text-right">
                    <Time value={op.created_at} />
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </section>
  );
}
