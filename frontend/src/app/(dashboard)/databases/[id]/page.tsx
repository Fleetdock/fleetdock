"use client";

import Link from "next/link";
import {
  useParams,
  usePathname,
  useRouter,
  useSearchParams,
} from "next/navigation";
import { Suspense, useEffect, useState } from "react";

import {
  ConnectivitySection,
  CredentialsSection,
} from "@/components/connectivity";
import { DatabaseBackups } from "@/components/backup/database-backups";
import { GrantsSection } from "@/components/database/grants-section";
import { SchemaPrivilegesSection } from "@/components/database/schema-privileges-section";
import { SchemaHistorySection } from "@/components/database/schema-history";
import { DatabaseOverview } from "@/components/database/overview";
import { ImportSQLModal } from "@/components/database/import-sql-modal";
import { MoveDatabaseModal } from "@/components/database/move-database-modal";
import { DeleteDatabaseModal } from "@/components/delete-database-modal";
import {
  Badge,
  Button,
  ButtonLink,
  Callout,
  HeaderSkeleton,
  Menu,
  PageHeader,
  QueryError,
  QueryTabs,
  Skeleton,
  StatStripSkeleton,
  StatTile,
  StatusBadge,
  Time,
} from "@/components/ui";
import { formatBytes as formatBytesOr } from "@/lib/format";

import { useCan, useCanOn, useDatabase, useDBObjects, useTables } from "@/lib/hooks";

import {
  Activity,
  ArrowRightLeft,
  Eye,
  FileUp,
  HardDrive,
  Network,
  ScrollText,
  Table2,
  Trash2,
} from "lucide-react";

const formatBytes = (n: number) => formatBytesOr(n, "0 B");

const TAB_LABEL = {
  overview: "Overview",
  connection: "Connection",
  backups: "Backups",
  users: "Access",
  schema: "Schema",
} as const;
type Tab = keyof typeof TAB_LABEL;
// These read the database itself, so they need the server's admin login.
const NEEDS_LOGIN: Tab[] = ["users", "schema"];

export default function DatabaseDetailPage() {
  return (
    <Suspense fallback={<DatabaseDetailSkeleton />}>
      <DatabaseDetail />
    </Suspense>
  );
}

function DatabaseDetail() {
  const params = useParams();
  const id = String(params.id);
  const { data: db, isLoading, error, refetch } = useDatabase(id);
  // The owning instance is embedded in the database response — no second
  // request, and no window where the page renders with the instance unknown.
  const instance = db?.instance;
  // The API authorizes these routes per-resource (requireResourcePerm), so a
  // user holding only a database- or server-scoped grant must not see the UI
  // disabled for writes the server would accept.
  const canOn = useCanOn();
  const scope = { databaseId: id, serverId: instance?.server_id ?? undefined };
  const canWrite = canOn("database:write", scope);
  const canMove = canOn("backup:write", scope);
  const canAudit = useCan()("audit:read");
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [moveOpen, setMoveOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);

  const router = useRouter();
  const pathname = usePathname();
  const sp = useSearchParams();
  const tabParam = sp.get("tab");
  const requested: Tab = tabParam && tabParam in TAB_LABEL ? (tabParam as Tab) : "overview";

  // Tables and the SQL console moved to the Data Browser; old links
  // (?tab=tables|query, ?schema=, ?table=) land there instead.
  const legacyTab = sp.get("tab");
  const legacyTable = sp.get("table");
  const legacy = legacyTab === "tables" || legacyTab === "query" || sp.has("schema") || legacyTable !== null;
  // The diagram has its own full-screen page now.
  useEffect(() => {
    if (legacyTab === "diagram") router.replace(`/databases/${encodeURIComponent(id)}/diagram`);
  }, [legacyTab, id, router]);
  useEffect(() => {
    if (!legacy) return;
    const q = new URLSearchParams({ db: id });
    if (legacyTable) q.set("table", legacyTable);
    else if (legacyTab === "query") q.set("query", "new");
    router.replace(`/data?${q.toString()}`);
  }, [legacy, legacyTab, legacyTable, id, router]);

  if (isLoading) {
    return <DatabaseDetailSkeleton />;
  }
  if (!db) {
    return (
      <QueryError
        error={error}
        what="database"
        onRetry={() => void refetch()}
        backHref="/databases"
        backLabel="Back to databases"
      />
    );
  }

  const hasCreds = instance?.has_credentials ?? false;
  const dbParam = encodeURIComponent(id);
  const browserHref = `/data?db=${dbParam}`;
  const tab: Tab = !hasCreds && NEEDS_LOGIN.includes(requested) ? "overview" : requested;
  const tabHref = (t: string) => (t === "overview" ? pathname : `${pathname}?tab=${t}`);

  return (
    <div>
      <PageHeader
        breadcrumbs={[
          { label: "Databases", href: "/databases" },
          ...(instance ? [{ label: instance.name, href: `/instances/${instance.id}` }] : []),
          { label: db.name },
        ]}
        title={db.name}
        badges={
          <>
            <StatusBadge status={db.status} />
            {db.system ? (
              <Badge title="Belongs to the database engine itself. You can browse and back it up, but not remove it.">
                system
              </Badge>
            ) : null}
          </>
        }
        actions={
          <>
            {hasCreds ? (
              <ButtonLink href={`/databases/${dbParam}/diagram`} icon={<Network size={15} aria-hidden />}>
                Diagram
              </ButtonLink>
            ) : null}
            {hasCreds ? (
              <ButtonLink
                variant="primary"
                href={browserHref}
                icon={<Table2 size={15} aria-hidden />}
              >
                Open Data Browser
              </ButtonLink>
            ) : null}
            {(canWrite && !db.system) || canAudit || (canMove && !db.system) ? (
              <Menu
                label="More database actions"
                items={[
                  {
                    label: "Audit log",
                    icon: <ScrollText size={15} />,
                    onSelect: () =>
                      router.push(
                        `/activity/audit?${new URLSearchParams({ resource_id: id, resource: db.name, period: "all" }).toString()}`,
                      ),
                    hidden: !canAudit,
                  },
                  {
                    label: "Copy or move",
                    icon: <ArrowRightLeft size={15} />,
                    onSelect: () => setMoveOpen(true),
                    hidden: !(canMove && !db.system),
                  },
                  {
                    label: "Import SQL",
                    icon: <FileUp size={15} />,
                    onSelect: () => setImportOpen(true),
                    hidden: !(canWrite && !db.system && canMove && hasCreds),
                  },
                  {
                    label: "Remove database",
                    icon: <Trash2 size={15} />,
                    onSelect: () => setDeleteOpen(true),
                    danger: true,
                    hidden: !(canWrite && !db.system),
                  },
                ]}
              />
            ) : null}
          </>
        }
      />

      {db.status === "missing" ? (
        <Callout
          tone="warning"
          role="status"
          title={
            <>
              This database can no longer be found on its server
              {db.missing_since ? (
                <>
                  {" "}(since <Time value={db.missing_since} />)
                </>
              ) : null}
            </>
          }
        >
          It may have been dropped or renamed outside Fleetdock. Its backups are kept, and it becomes active again
          automatically if it reappears. Remove it here if it is gone for good.
        </Callout>
      ) : null}

      <SummaryStrip
        databaseId={id}
        sizeBytes={db.size_bytes}
        connections={db.active_connections}
        hasCreds={hasCreds}
        browserHref={browserHref}
      />

      {!hasCreds ? (
        <Callout tone="info" title="Fleetdock can't log in to this database server yet.">
          Add an admin login on{" "}
          {instance ? (
            <Link href={`/instances/${instance.id}`} className="link">
              {instance.name}
            </Link>
          ) : (
            "the database server's page"
          )}{" "}
          to browse tables, see the diagram and schema history, and manage access.
        </Callout>
      ) : null}

      <section className="section">
        <QueryTabs
          label="Database sections"
          panelId="database-tabpanel"
          // replace, not push: arrow keys move between tabs, and each
          // move must not leave a history entry.
          onSelect={(t) => router.replace(tabHref(t), { scroll: false })}
          tabs={[
            { id: "overview", label: "Overview" },
            { id: "connection", label: "Connection" },
            { id: "backups", label: "Backups" },
            { id: "users", label: "Access", hidden: !hasCreds },
            { id: "schema", label: "Schema", hidden: !hasCreds },
          ]}
        />

        <div id="database-tabpanel" role="tabpanel" aria-label={TAB_LABEL[tab]} className="db-tab-panel">
          {tab === "overview" ? (
            <DatabaseOverview database={db} hasCreds={hasCreds} canBackup={canMove} tabHref={tabHref} />
          ) : tab === "connection" ? (
            <>
              <ConnectivitySection databaseId={id} databaseName={db.name} canWrite={canWrite} />
              <CredentialsSection databaseId={id} canWrite={canWrite} />
            </>
          ) : tab === "backups" ? (
            <DatabaseBackups database={db} canBackup={canMove} />
          ) : tab === "schema" ? (
            <SchemaHistorySection database={db} canWrite={canWrite} />
          ) : (
            <>
              <GrantsSection databaseId={id} canWrite={canWrite} />
              {instance?.engine === "postgres" && hasCreds ? (
                <SchemaPrivilegesSection
                  databaseId={id}
                  canWrite={canWrite}
                  canManage={canWrite && canOn("instance:write", scope)}
                />
              ) : null}
            </>
          )}
        </div>
      </section>

      <DeleteDatabaseModal
        database={deleteOpen ? db : null}
        onClose={() => setDeleteOpen(false)}
        onDeleted={() => router.push("/databases")}
      />
      <MoveDatabaseModal
        database={moveOpen ? db : null}
        onClose={() => setMoveOpen(false)}
      />
      {importOpen ? (
        <ImportSQLModal
          databaseId={id}
          databaseName={db.name}
          onClose={() => setImportOpen(false)}
        />
      ) : null}
    </div>
  );
}

/**
 * SummaryStrip shows the database's headline figures. Object counts need the
 * server's admin login, so without one those tiles say so instead.
 */
function SummaryStrip({
  databaseId,
  sizeBytes,
  connections,
  hasCreds,
  browserHref,
}: {
  databaseId: string;
  sizeBytes: number;
  connections: number;
  hasCreds: boolean;
  browserHref: string;
}) {
  const tables = useTables(hasCreds ? databaseId : "");
  const objects = useDBObjects(hasCreds ? databaseId : "");
  const nTables = tables.data?.items.length;
  const nViews = objects.data?.filter((o) => o.kind === "view" || o.kind === "materialized_view").length;
  const nOther = objects.data ? objects.data.length - (nViews ?? 0) : undefined;
  const count = (n: number | undefined, loading: boolean) =>
    !hasCreds ? "—" : loading || n === undefined ? <Skeleton width="2.5rem" height="1.2rem" /> : n.toLocaleString();

  return (
    <div className="stat-strip">
      <StatTile label="Size" icon={<HardDrive size={13} aria-hidden />} value={sizeBytes ? formatBytes(sizeBytes) : "—"} />
      <StatTile
        label="Tables"
        icon={<Table2 size={13} aria-hidden />}
        value={count(nTables, tables.isLoading)}
        hint={hasCreds ? "Open in Data Browser" : "Needs a server login"}
        href={hasCreds ? browserHref : undefined}
      />
      <StatTile
        label="Views"
        icon={<Eye size={13} aria-hidden />}
        value={count(nViews, objects.isLoading)}
        hint={hasCreds && nOther ? `${nOther.toLocaleString()} other object${nOther === 1 ? "" : "s"}` : undefined}
        href={hasCreds ? browserHref : undefined}
      />
      <StatTile label="Connections" icon={<Activity size={13} aria-hidden />} value={connections.toLocaleString()} hint="Open right now" />
    </div>
  );
}

/** DatabaseDetailSkeleton mirrors the page layout while the database loads: header, figures, tabs, cards. */
function DatabaseDetailSkeleton() {
  return (
    <div role="status" aria-label="Loading database">
      <HeaderSkeleton breadcrumbs />
      <StatStripSkeleton />
      <div className="flex gap-6 mb-4" aria-hidden>
        {[5, 6.5, 4.5, 4, 4].map((w, i) => (
          <Skeleton key={i} width={`${w}rem`} height="1.1rem" />
        ))}
      </div>
      <div className="db-overview" aria-hidden>
        {[0, 1].map((i) => (
          <div key={i} className="card panel stack-sm">
            <Skeleton width="30%" height=".9rem" />
            <Skeleton width="80%" />
            <Skeleton width="65%" />
            <Skeleton width="72%" />
          </div>
        ))}
      </div>
    </div>
  );
}
