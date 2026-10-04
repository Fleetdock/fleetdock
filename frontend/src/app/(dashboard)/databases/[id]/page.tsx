"use client";

import Link from "next/link";
import {
  useParams,
  usePathname,
  useRouter,
  useSearchParams,
} from "next/navigation";
import { Suspense, useState } from "react";

import {
  ConnectivitySection,
  CredentialsSection,
} from "@/components/connectivity";
import { DatabaseBackups } from "@/components/backup/database-backups";
import { DataBrowser } from "@/components/database/data-browser";
import { GrantsSection } from "@/components/database/grants-section";
import { ImportSQLModal } from "@/components/database/import-sql-modal";
import { MoveDatabaseModal } from "@/components/database/move-database-modal";
import { QueryConsole } from "@/components/database/query-console";
import { TablesBrowser } from "@/components/database/tables-browser";
import { DeleteDatabaseModal } from "@/components/delete-database-modal";
import { EmptyState, PageHeader, QueryTabs, Spinner, StatusBadge, Time } from "@/components/ui";
import { formatBytes as formatBytesOr } from "@/lib/format";

import { useCanOn, useDatabase } from "@/lib/hooks";

import { ArrowRightLeft, FileUp, Table2, Trash2 } from "lucide-react";

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

  // View state lives in the URL so the browser back button steps
  // data browser -> tables list -> schema list -> databases list.
  const router = useRouter();
  const pathname = usePathname();
  const sp = useSearchParams();
  const tabParam = sp.get("tab");
  const tab =
    tabParam === "users" || tabParam === "query" || tabParam === "backups"
      ? tabParam
      : "tables";
  const schema = sp.get("schema");
  const table = sp.get("table");
  const page = Math.max(1, parseInt(sp.get("page") ?? "1", 10) || 1);

  function navigate(updates: Record<string, string | null>) {
    const q = new URLSearchParams(sp.toString());
    for (const [k, v] of Object.entries(updates)) {
      if (v === null) {
        q.delete(k);
      } else {
        q.set(k, v);
      }
    }
    const qs = q.toString();
    router.push(qs ? `${pathname}?${qs}` : pathname);
  }

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
          {hasCreds ? (
            <Link
              className="btn btn-sm btn-primary"
              href={`/data?db=${encodeURIComponent(id)}${table ? `&table=${encodeURIComponent(table)}` : ""}`}
            >
              <Table2 size={15} /> Open in Data Browser
            </Link>
          ) : null}
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

      <QueryTabs
        label="Database sections"
        onSelect={(t) =>
          navigate({ tab: t === "tables" ? null : t, schema: null, table: null, page: null })
        }
        tabs={[
          { id: "tables", label: "Tables" },
          { id: "query", label: "SQL console" },
          { id: "backups", label: "Backups" },
          { id: "users", label: "Access" },
        ]}
      />

      {tab === "backups" ? (
        <DatabaseBackups database={db} canBackup={canMove} />
      ) : !hasCreds ? (
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
      ) : tab === "query" ? (
        <QueryConsole databaseId={id} canWrite={canWrite} />
      ) : tab === "users" ? (
        <GrantsSection databaseId={id} canWrite={canWrite} />
      ) : table ? (
        <DataBrowser
          databaseId={id}
          table={table}
          page={page}
          canWrite={canWrite}
          onPage={(p) => navigate({ page: p <= 1 ? null : String(p) })}
          onClose={() => navigate({ table: null, page: null })}
          onRenamed={(t) => navigate({ table: t, page: null })}
        />
      ) : (
        <TablesBrowser
          databaseId={id}
          schema={schema}
          onOpenSchema={(s) => navigate({ schema: s, table: null, page: null })}
          onCloseSchema={() =>
            navigate({ schema: null, table: null, page: null })
          }
          onOpenTable={(t) => navigate({ table: t, page: null })}
          canWrite={canWrite}
        />
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
