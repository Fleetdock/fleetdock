"use client";

import Link from "next/link";
import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense, useMemo, useRef, useState, type FormEvent } from "react";

import { ArrowRightLeft, ChevronRight, Download, KeyRound, Play, Plus, Table2, TerminalSquare, Trash2, X } from "lucide-react";
import { DeleteDatabaseModal } from "@/components/delete-database-modal";
import { ConnectivitySection, CredentialsSection } from "@/components/connectivity";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { SqlEditor, type SqlEditorHandle } from "@/components/sql-editor";
import { EmptyState, ErrorText, Field, Modal, Pagination, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  exportQueryCSV,
  exportTableCSV,
  useCanOn,
  useDatabase,
  useDatabaseDBUsers,
  useDBPrivileges,
  useDestinations,
  useGrantOnDatabase,
  useInstance,
  useInstances,
  useRevokeOnDatabase,
  useRunQuery,
  useSchemaGrants,
  useStartMove,
  useTableRows,
  useTables,
  useTableSchema,
} from "@/lib/hooks";
import { useDataTable } from "@/lib/use-data-table";
import type { Database, QueryResult, SchemaGrant, TableInfo } from "@/lib/types";

const PAGE_SIZE = 50;

function formatBytes(n: number) {
  if (!n) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let u = 0;
  while (v >= 1024 && u < units.length - 1) {
    v /= 1024;
    u++;
  }
  return `${v.toFixed(u === 0 ? 0 : 1)} ${units[u]}`;
}

export default function DatabaseDetailPage() {
  return (
    <Suspense fallback={<div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>}>
      <DatabaseDetail />
    </Suspense>
  );
}

function DatabaseDetail() {
  const params = useParams();
  const id = String(params.id);
  const { data: db, isLoading } = useDatabase(id);
  const { data: instance } = useInstance(db?.instance_id ?? "");
  // The API authorizes these routes per-resource (requireResourcePerm), so a
  // user holding only a database- or server-scoped grant must not see the UI
  // disabled for writes the server would accept.
  const canOn = useCanOn();
  const scope = { databaseId: id, serverId: instance?.server_id };
  const canWrite = canOn("database:write", scope);
  const canMove = canOn("backup:write", scope);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [moveOpen, setMoveOpen] = useState(false);

  // View state lives in the URL so the browser back button steps
  // data browser -> tables list -> databases list.
  const router = useRouter();
  const pathname = usePathname();
  const sp = useSearchParams();
  const tabParam = sp.get("tab");
  const tab = tabParam === "users" ? "users" : tabParam === "query" ? "query" : "tables";
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
    return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>;
  }
  if (!db) {
    return <EmptyState title="Database not found" />;
  }

  const hasCreds = instance?.has_credentials ?? false;

  return (
    <div>
      <Link href="/databases" className="muted text-sm">← Databases</Link>
      <div className="flex items-center justify-between" style={{ margin: ".6rem 0 1.1rem" }}>
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold">{db.name}</h1>
          <StatusBadge status={db.status} />
          {db.system ? (
            <span
              className="badge badge-gray"
              title="Engine-owned database — browsable and backup-able, not removable"
            >
              system
            </span>
          ) : null}
        </div>
        <div className="flex items-center gap-2">
          {canMove && !db.system ? (
            <button className="btn btn-sm" onClick={() => setMoveOpen(true)}>
              <ArrowRightLeft size={15} /> Move
            </button>
          ) : null}
          {canWrite && !db.system ? (
            <button className="btn btn-sm btn-danger" onClick={() => setDeleteOpen(true)}>
              <Trash2 size={15} /> Remove
            </button>
          ) : null}
        </div>
      </div>

      <div className="card" style={{ padding: "1.1rem", marginBottom: "1.25rem" }}>
        <dl className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))", gap: "1rem" }}>
          <Detail label="Instance" value={instance?.name ?? db.instance_id.slice(0, 8)} link={instance ? `/instances/${instance.id}` : undefined} />
          <Detail label="Charset" value={`${db.charset} / ${db.collation}`} />
          <Detail label="Size" value={db.size_bytes ? formatBytes(db.size_bytes) : "—"} />
          <Detail label="Created" value={new Date(db.created_at).toLocaleString()} />
        </dl>
      </div>

      <ConnectivitySection databaseId={id} canWrite={canWrite} />
      <CredentialsSection databaseId={id} canWrite={canWrite} />

      <div className="flex items-center gap-2" style={{ marginBottom: ".9rem" }}>
        <button
          className={`btn btn-sm${tab === "tables" ? " btn-primary" : ""}`}
          onClick={() => navigate({ tab: null, table: null, page: null })}
        >
          Tables
        </button>
        <button
          className={`btn btn-sm${tab === "query" ? " btn-primary" : ""}`}
          onClick={() => navigate({ tab: "query", table: null, page: null })}
        >
          <TerminalSquare size={15} /> SQL console
        </button>
        <button
          className={`btn btn-sm${tab === "users" ? " btn-primary" : ""}`}
          onClick={() => navigate({ tab: "users", table: null, page: null })}
        >
          Users & grants
        </button>
      </div>

      {!hasCreds ? (
        <EmptyState
          title="Live browsing unavailable"
          hint="Add admin credentials to the instance to browse tables and manage grants."
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
          onPage={(p) => navigate({ page: p <= 1 ? null : String(p) })}
          onClose={() => navigate({ table: null, page: null })}
        />
      ) : (
        <TablesSection databaseId={id} onOpen={(t) => navigate({ table: t, page: null })} />
      )}
      <DeleteDatabaseModal
        database={deleteOpen ? db : null}
        instance={instance}
        onClose={() => setDeleteOpen(false)}
        onDeleted={() => router.push("/databases")}
      />
      <MoveDatabaseModal database={moveOpen ? db : null} onClose={() => setMoveOpen(false)} />
    </div>
  );
}

function MoveDatabaseModal({ database, onClose }: { database: Database | null; onClose: () => void }) {
  const start = useStartMove();
  const { data: instances } = useInstances();
  const { data: destinations } = useDestinations();
  const [targetInstance, setTargetInstance] = useState("");
  const [targetName, setTargetName] = useState("");
  const [destination, setDestination] = useState("");
  const [dropSource, setDropSource] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [operationId, setOperationId] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!database) return;
    setError(null);
    try {
      const res = await start.mutateAsync({
        source_database_id: database.id,
        target_instance_id: targetInstance,
        target_database: targetName || undefined,
        destination_id: destination,
        drop_source: dropSource,
      });
      setOperationId(res.operation_id);
      setNotice("Move started — it will back up, then restore and verify in the background. Track it in Operations.");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to start move");
    }
  }

  if (!database) return null;
  return (
    <Modal open onClose={onClose} title={`Move "${database.name}"`}>
      {notice ? (
        <div>
          <div className="card" style={{ padding: ".8rem .9rem", marginBottom: "1rem" }}>
            <span className="text-sm">{notice}</span>
          </div>
          <div className="flex items-center justify-end gap-2">
            <Link href={operationId ? `/operations/${operationId}` : "/operations"} className="btn btn-sm">View operation</Link>
            <button className="btn btn-primary btn-sm" onClick={onClose}>Done</button>
          </div>
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          <Field label="Target instance">
            <select className="input" value={targetInstance} onChange={(e) => setTargetInstance(e.target.value)} required>
              <option value="">Select an instance…</option>
              {(instances?.items ?? [])
                .filter((i) => i.id !== database.instance_id && i.has_credentials)
                .map((i) => (
                  <option key={i.id} value={i.id}>{i.name} ({i.engine})</option>
                ))}
            </select>
          </Field>
          <Field label="Target database name (optional)">
            <input className="input" value={targetName} onChange={(e) => setTargetName(e.target.value)} placeholder={database.name} />
          </Field>
          <Field label="Backup destination">
            <select className="input" value={destination} onChange={(e) => setDestination(e.target.value)} required>
              <option value="">Select a destination…</option>
              {(destinations?.items ?? []).map((d) => (
                <option key={d.id} value={d.id}>{d.name}</option>
              ))}
            </select>
          </Field>
          <label className="flex items-center gap-2 text-sm" style={{ cursor: "pointer", margin: ".2rem 0 .6rem" }}>
            <input type="checkbox" checked={dropSource} onChange={(e) => setDropSource(e.target.checked)} />
            Drop the source database after a successful, verified move (cutover)
          </label>
          <p className="muted text-sm">
            The move backs up the source, restores it to the target (verifying the checksum and table count), then
            optionally drops the source. It runs in the background.
          </p>
          <ErrorText message={error ?? undefined} />
          <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
            <button type="button" className="btn" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={start.isPending}>
              {start.isPending ? "Starting…" : "Start move"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}

function Detail({ label, value, link }: { label: string; value: string; link?: string }) {
  return (
    <div>
      <dt className="muted text-sm">{label}</dt>
      <dd className="font-medium" style={{ margin: 0 }}>
        {link ? <Link href={link} style={{ textDecoration: "underline" }}>{value}</Link> : value}
      </dd>
    </div>
  );
}

// ---- Tables list ----

function compareTables(a: TableInfo, b: TableInfo, key: string) {
  if (key === "name" || key === "engine" || key === "schema") {
    return (a[key] ?? "").localeCompare(b[key] ?? "");
  }
  return (a[key as "row_count" | "data_bytes" | "index_bytes"] as number)
    - (b[key as "row_count" | "data_bytes" | "index_bytes"] as number);
}

function TablesSection({ databaseId, onOpen }: { databaseId: string; onOpen: (table: string) => void }) {
  const { data, isLoading, error } = useTables(databaseId);
  const [search, setSearch] = useState("");

  // A PostgreSQL database can spread its tables over several schemas, in which
  // case a bare table name is ambiguous: show the schema and address tables as
  // "schema.table". MySQL/MariaDB always report a single schema (the database),
  // so the column stays hidden and names stay bare.
  const schemas = useMemo(
    () => new Set((data?.items ?? []).map((t) => t.schema).filter(Boolean)),
    [data],
  );
  const multiSchema = schemas.size > 1;
  const qualify = (t: TableInfo) => (multiSchema ? `${t.schema}.${t.name}` : t.name);

  const table = useDataTable({
    items: data?.items,
    search: {
      query: search,
      match: (t, q) =>
        t.name.toLowerCase().includes(q) ||
        (multiSchema && qualify(t).toLowerCase().includes(q)),
    },
    sort: {
      key: "name",
      dir: "asc",
      compare: compareTables,
      defaultDir: (key) => (key === "name" || key === "engine" || key === "schema" ? "asc" : "desc"),
    },
  });

  return (
    <DataTable<TableInfo>
      columns={[
        {
          id: "name",
          header: "Table",
          sortable: true,
          sortKey: "name",
          className: "font-medium flex items-center gap-2",
          render: (t) => (
            <>
              <Table2 size={15} /> {t.name}
            </>
          ),
        },
        ...(multiSchema
          ? [
              {
                id: "schema",
                header: "Schema",
                sortable: true,
                sortKey: "schema",
                className: "muted",
                render: (t: TableInfo) => t.schema,
              } satisfies DataTableColumn<TableInfo>,
            ]
          : []),
        { id: "engine", header: "Engine", sortable: true, sortKey: "engine", className: "muted", render: (t) => t.engine },
        {
          id: "row_count",
          header: "Rows (est.)",
          sortable: true,
          sortKey: "row_count",
          className: "muted",
          render: (t) => t.row_count.toLocaleString(),
        },
        {
          id: "data_bytes",
          header: "Data",
          sortable: true,
          sortKey: "data_bytes",
          className: "muted",
          render: (t) => formatBytes(t.data_bytes),
        },
        {
          id: "index_bytes",
          header: "Indexes",
          sortable: true,
          sortKey: "index_bytes",
          className: "muted",
          render: (t) => formatBytes(t.index_bytes),
        },
        {
          id: "actions",
          header: "",
          align: "right",
          render: (t) => (
            <button className="btn btn-ghost btn-sm" onClick={() => onOpen(qualify(t))}>
              Browse <ChevronRight size={15} />
            </button>
          ),
        },
      ]}
      rows={table.rows}
      rowKey={(t) => `${t.schema}.${t.name}`}
      isLoading={isLoading}
      loadingLabel="Connecting to instance…"
      error={error ? (error as ApiError).message : undefined}
      errorTitle="Could not reach the instance"
      emptyTitle="No tables"
      emptyHint="This database has no base tables yet."
      emptySearchTitle="No tables match your search"
      emptySearchHint="Try a different name or clear the search."
      search={{
        value: search,
        onChange: (v) => {
          setSearch(v);
          table.setPage(1);
        },
        placeholder: "Search tables…",
      }}
      sort={{ key: table.sortKey, dir: table.sortDir, onSort: table.setSort }}
      pagination={{ page: table.page, pageCount: table.pageCount, onPage: table.setPage }}
    />
  );
}

// ---- Data browser with numbered pagination ----

function DataBrowser({
  databaseId,
  table,
  page,
  onPage,
  onClose,
}: {
  databaseId: string;
  table: string;
  page: number;
  onPage: (page: number) => void;
  onClose: () => void;
}) {
  const [view, setView] = useState<"data" | "schema">("data");
  const offset = (page - 1) * PAGE_SIZE;
  const { data, isLoading, error, isFetching } = useTableRows(databaseId, table, PAGE_SIZE, offset);

  // TABLE_ROWS is an estimate: trust it for page count, but always allow
  // "next" while full pages keep coming back.
  const hasFullPage = (data?.rows.length ?? 0) === PAGE_SIZE;
  const estimatedPages = data && data.total > 0 ? Math.ceil(data.total / PAGE_SIZE) : 0;
  const pageCount = Math.max(estimatedPages, hasFullPage ? page + 1 : page);

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: ".6rem", flexWrap: "wrap", gap: ".5rem" }}>
        <div className="flex items-center gap-2">
          <button className="btn btn-ghost btn-sm" onClick={onClose} aria-label="Back to tables">
            <X size={15} />
          </button>
          <span className="font-semibold flex items-center gap-2"><Table2 size={15} /> {table}</span>
          {view === "data" && data ? (
            <span className="muted text-sm">
              rows {offset + 1}–{offset + data.rows.length}
              {data.total > 0 ? ` of ~${data.total.toLocaleString()}` : ""}
            </span>
          ) : null}
          {isFetching && view === "data" ? <Spinner /> : null}
        </div>
        <div className="flex items-center gap-2" style={{ flexWrap: "wrap" }}>
          <div className="flex items-center gap-1">
            <button className={`btn btn-sm${view === "data" ? " btn-primary" : ""}`} onClick={() => setView("data")}>Data</button>
            <button className={`btn btn-sm${view === "schema" ? " btn-primary" : ""}`} onClick={() => setView("schema")}>Schema</button>
          </div>
          <ExportButton label="Export CSV" run={() => exportTableCSV(databaseId, table)} />
          {view === "data" ? (
            <Pagination page={page} pageCount={pageCount} hasMore={hasFullPage} onPage={onPage} />
          ) : null}
        </div>
      </div>

      {view === "schema" ? (
        <SchemaView databaseId={databaseId} table={table} />
      ) : isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading rows…</div>
      ) : error ? (
        <EmptyState title="Could not load rows" hint={(error as ApiError).message} />
      ) : !data || data.columns.length === 0 ? (
        <EmptyState title="No data" />
      ) : (
        <>
          <div className="card" style={{ overflowX: "auto" }}>
            <table className="table" style={{ fontSize: 13, whiteSpace: "nowrap" }}>
              <thead>
                <tr>
                  {data.columns.map((c) => (
                    <th key={c}>{c}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {data.rows.length === 0 ? (
                  <tr>
                    <td colSpan={data.columns.length} className="muted">No rows on this page.</td>
                  </tr>
                ) : (
                  data.rows.map((row, ri) => (
                    <tr key={ri}>
                      {row.map((cell, ci) => (
                        <td key={ci} className={cell === null ? "muted" : undefined} style={{ maxWidth: 320, overflow: "hidden", textOverflow: "ellipsis" }} title={cell ?? undefined}>
                          {cell === null ? "NULL" : cell}
                        </td>
                      ))}
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
          <div className="flex items-center justify-end" style={{ marginTop: ".6rem" }}>
            <Pagination page={page} pageCount={pageCount} hasMore={hasFullPage} onPage={onPage} />
          </div>
        </>
      )}
    </div>
  );
}

// ExportButton triggers a CSV download, showing a spinner while streaming and
// surfacing any error inline (downloads have no natural error surface otherwise).
function ExportButton({ label, run }: { label: string; run: () => Promise<void>; }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onClick() {
    setBusy(true);
    setError(null);
    try {
      await run();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Export failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <span className="flex items-center gap-2">
      {error ? <span className="text-sm" style={{ color: "var(--danger, #c00)" }} title={error}>Export failed</span> : null}
      <button className="btn btn-sm" onClick={onClick} disabled={busy}>
        {busy ? <Spinner /> : <Download size={15} />} {label}
      </button>
    </span>
  );
}

// ---- Table schema (columns, indexes, DDL) ----

function SchemaView({ databaseId, table }: { databaseId: string; table: string }) {
  const { data, isLoading, error } = useTableSchema(databaseId, table);

  if (isLoading) {
    return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading schema…</div>;
  }
  if (error) {
    return <EmptyState title="Could not load schema" hint={(error as ApiError).message} />;
  }
  if (!data) {
    return <EmptyState title="No schema" />;
  }

  return (
    <div className="flex" style={{ flexDirection: "column", gap: "1.1rem" }}>
      <div>
        <p className="muted text-sm" style={{ marginBottom: ".4rem" }}>Columns</p>
        <div className="card" style={{ overflowX: "auto" }}>
          <table className="table" style={{ fontSize: 13, whiteSpace: "nowrap" }}>
            <thead>
              <tr>
                <th>Column</th>
                <th>Type</th>
                <th>Null</th>
                <th>Key</th>
                <th>Default</th>
                <th>Extra</th>
                <th>Comment</th>
              </tr>
            </thead>
            <tbody>
              {data.columns.map((c) => (
                <tr key={c.name}>
                  <td className="font-medium">{c.name}</td>
                  <td className="muted"><code>{c.type}</code></td>
                  <td className="muted">{c.nullable ? "YES" : "NO"}</td>
                  <td className="muted">{c.key ? <span className="badge badge-gray" style={{ fontSize: 11 }}>{c.key}</span> : "—"}</td>
                  <td className={c.default === null ? "muted" : undefined}>{c.default === null ? "NULL" : c.default}</td>
                  <td className="muted">{c.extra || "—"}</td>
                  <td className="muted">{c.comment || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <div>
        <p className="muted text-sm" style={{ marginBottom: ".4rem" }}>Indexes</p>
        {data.indexes.length === 0 ? (
          <p className="muted text-sm">No indexes.</p>
        ) : (
          <div className="card" style={{ overflowX: "auto" }}>
            <table className="table" style={{ fontSize: 13, whiteSpace: "nowrap" }}>
              <thead>
                <tr>
                  <th>Index</th>
                  <th>Columns</th>
                  <th>Unique</th>
                  <th>Type</th>
                </tr>
              </thead>
              <tbody>
                {data.indexes.map((idx) => (
                  <tr key={idx.name}>
                    <td className="font-medium">{idx.name}</td>
                    <td className="muted">{idx.columns.join(", ")}</td>
                    <td className="muted">{idx.unique ? "YES" : "NO"}</td>
                    <td className="muted">{idx.type}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {data.ddl ? (
        <div>
          <p className="muted text-sm" style={{ marginBottom: ".4rem" }}>DDL</p>
          <pre className="card" style={{ padding: ".9rem", overflowX: "auto", fontSize: 12, margin: 0 }}>
            <code>{data.ddl}</code>
          </pre>
        </div>
      ) : null}
    </div>
  );
}

// ---- SQL console ----

function QueryConsole({ databaseId, canWrite }: { databaseId: string; canWrite: boolean }) {
  const run = useRunQuery(databaseId);
  const editorRef = useRef<SqlEditorHandle>(null);
  const [sql, setSql] = useState("");
  const [ranSql, setRanSql] = useState("");
  const [result, setResult] = useState<QueryResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function onRun() {
    const trimmed = editorRef.current?.getQueryToRun() ?? sql.trim();
    if (!trimmed) return;
    setError(null);
    try {
      const res = await run.mutateAsync({ sql: trimmed, limit: 200 });
      setResult(res);
      setRanSql(trimmed);
    } catch (err) {
      setResult(null);
      setError(err instanceof ApiError ? err.message : "Query failed");
    }
  }

  const canExport = Boolean(result && result.columns.length > 0 && ranSql);

  return (
    <div>
      <p className="muted text-sm" style={{ marginBottom: ".5rem" }}>
        {canWrite
          ? "Run SQL against this database. Reads return up to 200 rows; write statements report affected rows."
          : "Run read-only SQL against this database (returns up to 200 rows). You do not have write access."}
      </p>
      <SqlEditor
        ref={editorRef}
        value={sql}
        onChange={setSql}
        onRun={() => void onRun()}
        placeholder="SELECT * FROM ..."
        minHeight="220px"
      />
      <div className="flex items-center justify-between" style={{ marginTop: ".5rem", flexWrap: "wrap", gap: ".5rem" }}>
        <span className="muted text-sm">
          <span className="kbd">⌘/Ctrl</span> + <span className="kbd">Enter</span> to run
          {sql.trim() ? " · selection runs when highlighted" : ""}
        </span>
        <div className="flex items-center gap-2">
          {sql.trim() ? (
            <button className="btn btn-sm" onClick={() => setSql("")} disabled={run.isPending}>
              Clear
            </button>
          ) : null}
          {canExport ? (
            <ExportButton label="Export CSV" run={() => exportQueryCSV(databaseId, ranSql)} />
          ) : null}
          <button className="btn btn-primary btn-sm" onClick={() => void onRun()} disabled={run.isPending || !sql.trim()}>
            {run.isPending ? <Spinner /> : <Play size={15} />} Run
          </button>
        </div>
      </div>

      <ErrorText message={error ?? undefined} />

      {result ? <QueryResultView result={result} /> : null}
    </div>
  );
}

function QueryResultView({ result }: { result: QueryResult }) {
  if (result.columns.length === 0) {
    return (
      <div className="card" style={{ padding: ".8rem .9rem", marginTop: ".9rem" }}>
        <span className="text-sm flex items-center gap-2">
          <KeyRound size={15} /> {result.rows_affected.toLocaleString()} row{result.rows_affected === 1 ? "" : "s"} affected
          <span className="muted">· {result.duration_ms} ms</span>
        </span>
      </div>
    );
  }
  return (
    <div style={{ marginTop: ".9rem" }}>
      <div className="flex items-center justify-between" style={{ marginBottom: ".5rem", flexWrap: "wrap", gap: ".4rem" }}>
        <span className="muted text-sm">
          {result.row_count.toLocaleString()} row{result.row_count === 1 ? "" : "s"} · {result.duration_ms} ms
          {result.truncated ? " · truncated to first 200" : ""}
        </span>
      </div>
      <div className="card" style={{ overflowX: "auto" }}>
        <table className="table" style={{ fontSize: 13, whiteSpace: "nowrap" }}>
          <thead>
            <tr>
              {result.columns.map((c, i) => (
                <th key={`${c}-${i}`}>{c}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {result.rows.length === 0 ? (
              <tr>
                <td colSpan={result.columns.length} className="muted">No rows returned.</td>
              </tr>
            ) : (
              result.rows.map((row, ri) => (
                <tr key={ri}>
                  {row.map((cell, ci) => (
                    <td key={ci} className={cell === null ? "muted" : undefined} style={{ maxWidth: 320, overflow: "hidden", textOverflow: "ellipsis" }} title={cell ?? undefined}>
                      {cell === null ? "NULL" : cell}
                    </td>
                  ))}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// ---- Users & grants for this database ----

function GrantsSection({ databaseId, canWrite }: { databaseId: string; canWrite: boolean }) {
  const { data, isLoading, error } = useSchemaGrants(databaseId);
  const revoke = useRevokeOnDatabase();
  const [grantOpen, setGrantOpen] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);

  async function onRevoke(user: string, host: string) {
    if (!confirm(`Revoke all privileges on this database from '${user}'@'${host}'?`)) return;
    setNotice(null);
    try {
      await revoke.mutateAsync({ databaseId, username: user, host });
    } catch (err) {
      setNotice(err instanceof ApiError ? err.message : "Failed to revoke");
    }
  }

  const columns: DataTableColumn<SchemaGrant>[] = [
    { id: "user", header: "User", className: "font-medium", render: (g) => g.user },
    { id: "host", header: "Host", className: "muted", render: (g) => <code>{g.host}</code> },
    {
      id: "privileges",
      header: "Privileges",
      render: (g) => (
        <div className="flex" style={{ flexWrap: "wrap", gap: ".25rem" }}>
          {g.privileges.map((p) => (
            <span key={p} className="badge badge-gray" style={{ fontSize: 11 }}>{p}</span>
          ))}
        </div>
      ),
    },
  ];
  if (canWrite) {
    columns.push({
      id: "actions",
      header: "Actions",
      align: "right",
      render: (g) => (
        <button className="btn btn-sm btn-danger" onClick={() => onRevoke(g.user, g.host)} disabled={revoke.isPending}>
          Revoke all
        </button>
      ),
    });
  }

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: ".6rem" }}>
        <p className="muted text-sm">Accounts with privileges on this database (live).</p>
        {canWrite ? (
          <button className="btn btn-primary btn-sm" onClick={() => setGrantOpen(true)}>
            <Plus size={15} /> Grant access
          </button>
        ) : null}
      </div>

      {notice ? (
        <div className="card" style={{ padding: ".7rem .9rem", marginBottom: ".9rem" }}>
          <span className="text-sm">{notice}</span>
        </div>
      ) : null}

      <DataTable<SchemaGrant>
        columns={columns}
        rows={data?.items ?? []}
        rowKey={(g) => `${g.user}@${g.host}`}
        isLoading={isLoading}
        loadingLabel="Connecting to instance…"
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not reach the instance"
        emptyTitle="No accounts have privileges on this database"
        emptyHint='Use "Grant access" to give a database user access.'
      />

      <GrantAccessModal open={grantOpen} onClose={() => setGrantOpen(false)} databaseId={databaseId} />
    </div>
  );
}

function GrantAccessModal({
  open,
  onClose,
  databaseId,
}: {
  open: boolean;
  onClose: () => void;
  databaseId: string;
}) {
  const grant = useGrantOnDatabase();
  const { data: users } = useDatabaseDBUsers(databaseId);
  const { data: privileges } = useDBPrivileges();
  const [account, setAccount] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set(["ALL PRIVILEGES"]));
  const [error, setError] = useState<string | null>(null);

  function toggle(p: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(p)) {
        next.delete(p);
      } else {
        next.add(p);
      }
      return next;
    });
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const [user, host] = account.split(" ");
    try {
      await grant.mutateAsync({ databaseId, username: user, host, privileges: [...selected] });
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to grant access");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Grant access to this database">
      <form onSubmit={onSubmit}>
        <Field label="Database user">
          <select className="input" value={account} onChange={(e) => setAccount(e.target.value)} required>
            <option value="" disabled>Select an account…</option>
            {(users?.items ?? []).map((u) => (
              <option key={`${u.user}@${u.host}`} value={`${u.user} ${u.host}`}>
                {u.user}@{u.host}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Privileges">
          <div className="card" style={{ padding: ".75rem", maxHeight: 220, overflowY: "auto" }}>
            <div className="flex" style={{ flexWrap: "wrap", gap: ".3rem 1rem" }}>
              {(privileges?.items ?? []).map((p) => (
                <label key={p} className="flex items-center gap-1 text-sm" style={{ cursor: "pointer" }}>
                  <input type="checkbox" checked={selected.has(p)} onChange={() => toggle(p)} />
                  {p}
                </label>
              ))}
            </div>
          </div>
        </Field>
        <p className="muted text-sm">
          Create new accounts on the instance detail page; here you grant existing accounts access.
        </p>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={grant.isPending || selected.size === 0 || !account}>
            {grant.isPending ? "Granting…" : "Grant"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
