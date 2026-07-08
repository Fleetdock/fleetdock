"use client";

import Link from "next/link";
import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense, useState, type FormEvent } from "react";

import { ArrowRightLeft, ChevronRight, Plus, Table2, Trash2, X } from "lucide-react";
import { DeleteDatabaseModal } from "@/components/delete-database-modal";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { EmptyState, ErrorText, Field, Modal, Pagination, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useCan,
  useDatabase,
  useDatabaseDBUsers,
  useDBPrivileges,
  useDestinations,
  useGrantOnDatabase,
  useInstance,
  useInstances,
  useRevokeOnDatabase,
  useSchemaGrants,
  useStartMove,
  useTableRows,
  useTables,
} from "@/lib/hooks";
import { useDataTable } from "@/lib/use-data-table";
import type { Database, SchemaGrant, TableInfo } from "@/lib/types";

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
  const can = useCan();
  const canWrite = can("database:write");
  const canMove = can("backup:write");
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [moveOpen, setMoveOpen] = useState(false);

  // View state lives in the URL so the browser back button steps
  // data browser -> tables list -> databases list.
  const router = useRouter();
  const pathname = usePathname();
  const sp = useSearchParams();
  const tab = sp.get("tab") === "users" ? "users" : "tables";
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
        </div>
        <div className="flex items-center gap-2">
          {canMove ? (
            <button className="btn btn-sm" onClick={() => setMoveOpen(true)}>
              <ArrowRightLeft size={15} /> Move
            </button>
          ) : null}
          {canWrite ? (
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

      <div className="flex items-center gap-2" style={{ marginBottom: ".9rem" }}>
        <button
          className={`btn btn-sm${tab === "tables" ? " btn-primary" : ""}`}
          onClick={() => navigate({ tab: null, table: null, page: null })}
        >
          Tables
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
      ) : tab === "tables" ? (
        table ? (
          <DataBrowser
            databaseId={id}
            table={table}
            page={page}
            onPage={(p) => navigate({ page: p <= 1 ? null : String(p) })}
            onClose={() => navigate({ table: null, page: null })}
          />
        ) : (
          <TablesSection databaseId={id} onOpen={(t) => navigate({ table: t, page: null })} />
        )
      ) : (
        <GrantsSection databaseId={id} canWrite={can("database:write")} />
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
  if (key === "name" || key === "engine") {
    return a[key].localeCompare(b[key]);
  }
  return (a[key as "row_count" | "data_bytes" | "index_bytes"] as number)
    - (b[key as "row_count" | "data_bytes" | "index_bytes"] as number);
}

function TablesSection({ databaseId, onOpen }: { databaseId: string; onOpen: (table: string) => void }) {
  const { data, isLoading, error } = useTables(databaseId);
  const [search, setSearch] = useState("");
  const table = useDataTable({
    items: data?.items,
    search: { query: search, match: (t, q) => t.name.toLowerCase().includes(q) },
    sort: {
      key: "name",
      dir: "asc",
      compare: compareTables,
      defaultDir: (key) => (key === "name" || key === "engine" ? "asc" : "desc"),
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
            <button className="btn btn-ghost btn-sm" onClick={() => onOpen(t.name)}>
              Browse <ChevronRight size={15} />
            </button>
          ),
        },
      ]}
      rows={table.rows}
      rowKey={(t) => t.name}
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
          {data ? (
            <span className="muted text-sm">
              rows {offset + 1}–{offset + data.rows.length}
              {data.total > 0 ? ` of ~${data.total.toLocaleString()}` : ""}
            </span>
          ) : null}
          {isFetching ? <Spinner /> : null}
        </div>
        <Pagination page={page} pageCount={pageCount} hasMore={hasFullPage} onPage={onPage} />
      </div>

      {isLoading ? (
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
