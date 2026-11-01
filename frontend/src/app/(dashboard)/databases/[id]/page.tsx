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
import { SchemaHistorySection } from "@/components/database/schema-history";
import { IndexHealthCard } from "@/components/database/index-health";
import { ImportSQLModal } from "@/components/database/import-sql-modal";
import { MoveDatabaseModal } from "@/components/database/move-database-modal";
import { DeleteDatabaseModal } from "@/components/delete-database-modal";
import {
  Badge,
  Button,
  ButtonLink,
  Callout,
  DetailSkeleton,
  EmptyState,
  KeyValueList,
  Menu,
  PageHeader,
  QueryError,
  QueryTabs,
  Skeleton,
  StatTile,
  StatusBadge,
  Time,
} from "@/components/ui";
import { engineLabel } from "@/lib/engines";
import { formatBytes as formatBytesOr } from "@/lib/format";

import { useCanOn, useDatabase, useDBObjects, useTables } from "@/lib/hooks";

import {
  Activity,
  ArrowRightLeft,
  Database,
  Eye,
  FileUp,
  HardDrive,
  SquareTerminal,
  Table2,
  Trash2,
  Users,
} from "lucide-react";

const formatBytes = (n: number) => formatBytesOr(n, "0 B");

const TAB_LABEL = { backups: "Backups", users: "Access", schema: "Schema" } as const;

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
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [moveOpen, setMoveOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);

  const router = useRouter();
  const pathname = usePathname();
  const sp = useSearchParams();
  const tabParam = sp.get("tab");
  const tab = tabParam === "users" || tabParam === "schema" ? tabParam : "backups";

  // Tables and the SQL console moved to the Data Browser; old links
  // (?tab=tables|query, ?schema=, ?table=) land there instead.
  const legacyTab = sp.get("tab");
  const legacyTable = sp.get("table");
  const legacy = legacyTab === "tables" || legacyTab === "query" || sp.has("schema") || legacyTable !== null;
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
  const queryHref = `/data?db=${dbParam}&query=new`;

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
            {canMove && !db.system ? (
              <Button
                icon={<ArrowRightLeft size={15} aria-hidden />}
                onClick={() => setMoveOpen(true)}
              >
                Copy / move
              </Button>
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
            {canWrite && !db.system ? (
              <Menu
                label="More database actions"
                items={[
                  {
                    label: "Import SQL",
                    icon: <FileUp size={15} />,
                    onSelect: () => setImportOpen(true),
                    hidden: !(canMove && hasCreds),
                  },
                  {
                    label: "Remove database",
                    icon: <Trash2 size={15} />,
                    onSelect: () => setDeleteOpen(true),
                    danger: true,
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

      <div className="detail-grid">
        <div className="detail-main">
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
              to browse tables, import SQL and manage access.
            </Callout>
          ) : null}

          <ConnectivitySection databaseId={id} databaseName={db.name} canWrite={canWrite} />
          <CredentialsSection databaseId={id} canWrite={canWrite} />

          <section className="section">
            <QueryTabs
              label="Database sections"
              panelId="database-tabpanel"
              onSelect={(t) => router.push(t === "backups" ? pathname : `${pathname}?tab=${t}`)}
              tabs={[
                { id: "backups", label: "Backups" },
                { id: "users", label: "Access" },
                { id: "schema", label: "Schema" },
              ]}
            />

            <div id="database-tabpanel" role="tabpanel" aria-label={TAB_LABEL[tab]}>
              {tab === "backups" ? (
                <DatabaseBackups database={db} canBackup={canMove} />
              ) : !hasCreds ? (
                <NoLogin instance={instance} />
              ) : tab === "schema" ? (
                <SchemaHistorySection database={db} canWrite={canWrite} />
              ) : (
                <GrantsSection databaseId={id} canWrite={canWrite} />
              )}
            </div>
          </section>
        </div>

        <aside className="detail-aside" aria-label="Database details">
          <div className="card panel aside-card">
            <h2 className="panel-title">Details</h2>
            <KeyValueList
              items={[
                {
                  label: "Server",
                  value: instance ? (
                    <Link href={`/instances/${instance.id}`} className="link">
                      {instance.name}
                    </Link>
                  ) : (
                    db.instance_id.slice(0, 8)
                  ),
                },
                { label: "Engine", value: instance ? engineLabel(instance.engine) : "—" },
                { label: "Charset", value: db.charset || "—", mono: true },
                { label: "Collation", value: db.collation || "—", mono: true },
                { label: "Created", value: <Time value={db.created_at} /> },
                { label: "Updated", value: <Time value={db.updated_at} /> },
                { label: "Tags", value: db.tags.join(", "), hidden: db.tags.length === 0 },
              ]}
            />
          </div>

          {hasCreds ? <IndexHealthCard databaseId={id} browserHref={browserHref} /> : null}

          <div className="card panel aside-card">
            <h2 className="panel-title">Quick links</h2>
            <ul className="quick-links">
              {hasCreds ? (
                <>
                  <li>
                    <Link href={browserHref}>
                      <Table2 size={15} aria-hidden /> Browse tables and views
                    </Link>
                  </li>
                  <li>
                    <Link href={queryHref}>
                      <SquareTerminal size={15} aria-hidden /> New SQL query
                    </Link>
                  </li>
                </>
              ) : null}
              {instance ? (
                <li>
                  <Link href={`/instances/${instance.id}`}>
                    <Database size={15} aria-hidden /> Database server {instance.name}
                  </Link>
                </li>
              ) : null}
              <li>
                <Link href={`${pathname}?tab=users`}>
                  <Users size={15} aria-hidden /> Users and privileges
                </Link>
              </li>
            </ul>
          </div>
        </aside>
      </div>

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

type InstanceRefLike = { id: string; name: string } | undefined;

/** NoLogin explains that browsing needs the server's admin login first. */
function NoLogin({ instance }: { instance: InstanceRefLike }) {
  return (
    <EmptyState
      title="Fleetdock can't log in to this database server yet"
      hint="Add an admin login on the database server's page to browse tables and manage access."
      action={
        instance ? (
          <ButtonLink href={`/instances/${instance.id}`}>
            Open {instance.name}
          </ButtonLink>
        ) : undefined
      }
    />
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

/** DatabaseDetailSkeleton mirrors the page layout while the database loads. */
function DatabaseDetailSkeleton() {
  return <DetailSkeleton label="Loading database" />;
}
