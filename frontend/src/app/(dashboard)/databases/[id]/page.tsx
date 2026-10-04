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
import { ImportSQLModal } from "@/components/database/import-sql-modal";
import { MoveDatabaseModal } from "@/components/database/move-database-modal";
import { DeleteDatabaseModal } from "@/components/delete-database-modal";
import { EmptyState, PageHeader, QueryTabs, Spinner, StatusBadge, Time } from "@/components/ui";
import { formatBytes as formatBytesOr } from "@/lib/format";

import { useCanOn, useDatabase, useDBObjects, useTables } from "@/lib/hooks";

import { ArrowRightLeft, FileUp, SquareTerminal, Table2, Trash2 } from "lucide-react";

const formatBytes = (n: number) => formatBytesOr(n, "0 B");

export default function DatabaseDetailPage() {
  return (
    <Suspense
      fallback={
        <div className="flex items-center gap-2 text-sm muted">
          <Spinner /> Loading…
        </div>
      }
    >
      <DatabaseDetail />
    </Suspense>
  );
}

function DatabaseDetail() {
  const params = useParams();
  const id = String(params.id);
  const { data: db, isLoading } = useDatabase(id);
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
  const tab = sp.get("tab") === "users" ? "users" : "backups";

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
    return (
      <div className="flex items-center gap-2 text-sm muted">
        <Spinner /> Loading…
      </div>
    );
  }
  if (!db) {
    return <EmptyState title="Database not found" />;
  }

  const hasCreds = instance?.has_credentials ?? false;

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
              <span
                className="badge badge-gray"
                title="Belongs to the database engine itself. You can browse and back it up, but not remove it."
              >
                system
              </span>
            ) : null}
          </>
        }
        actions={
          <>
          {canMove && !db.system ? (
            <button className="btn btn-sm" onClick={() => setMoveOpen(true)}>
              <ArrowRightLeft size={15} /> Copy / move
            </button>
          ) : null}
          {canWrite && canMove && !db.system && hasCreds ? (
            <button className="btn btn-sm" onClick={() => setImportOpen(true)}>
              <FileUp size={15} /> Import SQL
            </button>
          ) : null}
          {canWrite && !db.system ? (
            <button
              className="btn btn-sm btn-danger"
              onClick={() => setDeleteOpen(true)}
            >
              <Trash2 size={15} /> Remove
            </button>
          ) : null}
          </>
        }
      />

      {db.status === "missing" ? (
        <div className="callout callout-warning" role="status">
          <strong>This database can no longer be found on its server</strong>
          {db.missing_since ? (
            <>
              {" "}(since <Time value={db.missing_since} />)
            </>
          ) : null}
          . It may have been dropped or renamed outside Fleetdock. Its backups are kept, and it becomes active again
          automatically if it reappears. Remove it here if it is gone for good.
        </div>
      ) : null}

      <div
        className="card"
        style={{ padding: "1.1rem", marginBottom: "1.25rem" }}
      >
        <dl
          className="grid"
          style={{
            gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))",
            gap: "1rem",
          }}
        >
          <Detail
            label="Database server"
            value={instance?.name ?? db.instance_id.slice(0, 8)}
            link={instance ? `/instances/${instance.id}` : undefined}
          />
          <Detail label="Charset" value={`${db.charset} / ${db.collation}`} />
          <Detail
            label="Size"
            value={db.size_bytes ? formatBytes(db.size_bytes) : "—"}
          />
          <Detail
            label="Created"
            value={<Time value={db.created_at} />}
          />
        </dl>
      </div>

      <ConnectivitySection databaseId={id} canWrite={canWrite} />
      <CredentialsSection databaseId={id} canWrite={canWrite} />

      <DataCard databaseId={id} hasCreds={hasCreds} instance={instance} />

      <QueryTabs
        label="Database sections"
        onSelect={(t) => router.push(t === "backups" ? pathname : `${pathname}?tab=${t}`)}
        tabs={[
          { id: "backups", label: "Backups" },
          { id: "users", label: "Access" },
        ]}
      />

      {tab === "backups" ? (
        <DatabaseBackups database={db} canBackup={canMove} />
      ) : hasCreds ? (
        <GrantsSection databaseId={id} canWrite={canWrite} />
      ) : (
        <NoLogin instance={instance} />
      )}
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

function Detail({
  label,
  value,
  link,
}: {
  label: string;
  value: React.ReactNode;
  link?: string;
}) {
  return (
    <div>
      <dt className="text-sm muted">{label}</dt>
      <dd className="font-medium" style={{ margin: 0 }}>
        {link ? (
          <Link href={link} style={{ textDecoration: "underline" }}>
            {value}
          </Link>
        ) : (
          value
        )}
      </dd>
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
          <Link href={`/instances/${instance.id}`} className="btn">
            Open {instance.name}
          </Link>
        ) : undefined
      }
    />
  );
}

/**
 * DataCard points to the Data Browser, where this database's tables, views,
 * routines and SQL console live.
 */
function DataCard({ databaseId, hasCreds, instance }: { databaseId: string; hasCreds: boolean; instance: InstanceRefLike }) {
  const tables = useTables(hasCreds ? databaseId : "");
  const objects = useDBObjects(hasCreds ? databaseId : "");
  if (!hasCreds) {
    return (
      <div className="section">
        <NoLogin instance={instance} />
      </div>
    );
  }
  const nTables = tables.data?.items.length;
  const nViews = objects.data?.filter((o) => o.kind === "view" || o.kind === "materialized_view").length;
  const nOther = objects.data ? objects.data.length - (nViews ?? 0) : undefined;
  const plural = (n: number, word: string) => `${n.toLocaleString()} ${word}${n === 1 ? "" : "s"}`;
  const counts = [
    nTables !== undefined ? plural(nTables, "table") : null,
    nViews ? plural(nViews, "view") : null,
    nOther ? plural(nOther, "other object") : null,
  ].filter(Boolean);
  const db = encodeURIComponent(databaseId);
  return (
    <div className="card data-card">
      <div className="data-card-icon">
        <Table2 size={18} />
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div className="font-medium">Data</div>
        <div className="text-sm muted">
          {tables.isLoading
            ? "Counting tables…"
            : counts.length
              ? `${counts.join(" · ")} — browse, edit and query them in the Data Browser.`
              : "Browse, edit and query this database in the Data Browser."}
        </div>
      </div>
      <div className="flex items-center gap-2" style={{ flexWrap: "wrap" }}>
        <Link className="btn btn-sm" href={`/data?db=${db}&query=new`}>
          <SquareTerminal size={15} /> New SQL query
        </Link>
        <Link className="btn btn-sm btn-primary" href={`/data?db=${db}`}>
          <Table2 size={15} /> Open Data Browser
        </Link>
      </div>
    </div>
  );
}
