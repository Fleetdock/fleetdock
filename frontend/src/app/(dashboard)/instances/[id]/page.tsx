"use client";

import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";

import { KeyRound } from "lucide-react";
import {
  CheckNowButton,
  DBUsersSection,
  EditInstanceModal,
  InstanceControls,
  InstanceDatabases,
  InstanceOverview,
  instanceAddress,
  kindLabel,
} from "@/components/instance";
import { HealthBadge, InstanceMonitoring } from "@/components/instance-monitoring";
import {
  Button,
  Callout,
  HeaderSkeleton,
  PageHeader,
  QueryError,
  QueryTabs,
  Skeleton,
  StatusBadge,
  TableSkeleton,
} from "@/components/ui";
import { engineLabel } from "@/lib/engines";
import { useCan, useDatabases, useInstance, useServers } from "@/lib/hooks";

const PANEL_ID = "instance-section-panel";

export default function InstanceDetailPage() {
  return (
    <Suspense fallback={<InstanceDetailSkeleton />}>
      <InstanceDetail />
    </Suspense>
  );
}

/** InstanceDetailSkeleton mirrors the page: header, details card, tabs and a table. */
function InstanceDetailSkeleton() {
  return (
    <div role="status" aria-label="Loading database server">
      <HeaderSkeleton breadcrumbs />
      <div className="card panel stack-sm section" aria-hidden>
        <Skeleton width="20%" height=".9rem" />
        <Skeleton width="60%" />
        <Skeleton width="45%" />
        <Skeleton width="52%" />
      </div>
      <TableSkeleton rows={5} columns={5} />
    </div>
  );
}

function InstanceDetail() {
  const params = useParams();
  const id = String(params.id);
  const router = useRouter();
  const pathname = usePathname();
  const sp = useSearchParams();
  const tab = sp.get("tab") ?? "databases";
  const { data: instance, isLoading, error, refetch } = useInstance(id);
  const { data: servers } = useServers();
  // Paginated: an instance with more databases than one page used to render a
  // silently truncated list with no indication there was more.
  const [dbPage, setDbPage] = useState(1);
  const { data: databases, isLoading: databasesLoading } = useDatabases({ instance_id: id, page: dbPage });
  const [editOpen, setEditOpen] = useState(false);
  const can = useCan();

  if (isLoading) return <InstanceDetailSkeleton />;
  if (!instance) {
    return (
      <QueryError
        error={error}
        what="database server"
        onRetry={() => void refetch()}
        backHref="/databases"
        backLabel="Back to databases"
      />
    );
  }

  const canWrite = can("instance:write");
  const serverName = servers?.items.find((s) => s.id === instance.server_id)?.name;
  const where = instanceAddress(instance, serverName);
  const addLogin = canWrite ? (
    <Button variant="primary" icon={<KeyRound size={15} aria-hidden />} onClick={() => setEditOpen(true)}>
      Add login
    </Button>
  ) : undefined;

  function selectTab(t: string) {
    const q = new URLSearchParams(sp.toString());
    if (t === "databases") q.delete("tab");
    else q.set("tab", t);
    router.replace(q.toString() ? `${pathname}?${q}` : pathname);
  }

  const tabs = [
    { id: "databases", label: `Databases${databases ? ` (${databases.pagination.total})` : ""}` },
    { id: "monitoring", label: "Monitoring", hidden: !instance.has_credentials },
    { id: "users", label: "Database users", hidden: !instance.has_credentials },
  ];
  const showMonitoring = tab === "monitoring" && instance.has_credentials;
  const showUsers = tab === "users" && instance.has_credentials;
  const panelLabel = showMonitoring ? "Monitoring" : showUsers ? "Database users" : "Databases";

  return (
    <div>
      <PageHeader
        breadcrumbs={[{ label: "Databases", href: "/databases" }, { label: instance.name }]}
        title={instance.name}
        badges={
          <>
            <StatusBadge status={instance.status} />
            {instance.has_credentials ? <HealthBadge health={instance.health} /> : null}
          </>
        }
        description={`${engineLabel(instance.engine)} ${instance.engine_version} · ${kindLabel(instance)} · ${where}`}
        actions={
          <>
            {/* The probe only needs instance:read, so viewers keep this button. */}
            {instance.has_credentials ? <CheckNowButton instance={instance} /> : null}
            {canWrite ? <InstanceControls instance={instance} onEdit={() => setEditOpen(true)} /> : null}
          </>
        }
      />

      {!instance.has_credentials ? (
        <Callout tone="warning" role="status" title="Fleetdock can't log in to this server yet." actions={addLogin}>
          Add an admin user and password (Edit) so it can find databases, manage users, take backups and show
          monitoring.
        </Callout>
      ) : null}

      <InstanceOverview instance={instance} address={where} canWrite={canWrite} />

      <QueryTabs label="Database server sections" onSelect={selectTab} tabs={tabs} panelId={PANEL_ID} />

      <div id={PANEL_ID} role="tabpanel" aria-label={panelLabel} className="mt-4">
        {showMonitoring ? (
          <InstanceMonitoring instanceId={id} canKill={canWrite} engine={instance.engine} />
        ) : showUsers ? (
          <DBUsersSection instanceId={id} canWrite={canWrite} databases={databases?.items ?? []} />
        ) : (
          <InstanceDatabases
            databases={databases}
            isLoading={databasesLoading}
            page={dbPage}
            onPage={setDbPage}
            hasCredentials={instance.has_credentials}
            emptyAction={instance.has_credentials ? undefined : addLogin}
          />
        )}
      </div>

      <EditInstanceModal instance={editOpen ? instance : null} onClose={() => setEditOpen(false)} />
    </div>
  );
}
