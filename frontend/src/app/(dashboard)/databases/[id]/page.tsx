"use client";

import Link from "next/link";
import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense, useMemo, useState, type FormEvent } from "react";

import { ArrowDown, ArrowUp, ChevronRight, Plus, Search, Table2, Trash2, X } from "lucide-react";
import { DeleteDatabaseModal } from "@/components/delete-database-modal";
import { EmptyState, ErrorText, Field, Modal, Pagination, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useCan,
  useClientPage,
  useDatabase,
  useDatabaseDBUsers,
  useDBPrivileges,
  useGrantOnDatabase,
  useInstance,
  useRevokeOnDatabase,
  useSchemaGrants,
  useTableRows,
  useTables,
} from "@/lib/hooks";
import type { TableInfo } from "@/lib/types";

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
  const [deleteOpen, setDeleteOpen] = useState(false);

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
        {canWrite ? (
          <button className="btn btn-sm btn-danger" onClick={() => setDeleteOpen(true)}>
            <Trash2 size={15} /> Remove
          </button>
        ) : null}
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
    </div>
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

type TableSortKey = "name" | "engine" | "row_count" | "data_bytes" | "index_bytes";
type SortDir = "asc" | "desc";

function compareTables(a: TableInfo, b: TableInfo, key: TableSortKey, dir: SortDir) {
  let cmp = 0;
  if (key === "name" || key === "engine") {
    cmp = a[key].localeCompare(b[key]);
  } else {
    cmp = a[key] - b[key];
  }
  return dir === "asc" ? cmp : -cmp;
}

function SortableHeader({
  label,
  sortKey,
  activeKey,
  dir,
  onSort,
}: {
  label: string;
  sortKey: TableSortKey;
  activeKey: TableSortKey;
  dir: SortDir;
  onSort: (key: TableSortKey) => void;
}) {
  const active = sortKey === activeKey;
  return (
    <th>
      <button
        type="button"
        className="btn btn-ghost btn-sm"
        style={{
          padding: 0,
          font: "inherit",
          color: "inherit",
          textTransform: "inherit",
          letterSpacing: "inherit",
          fontWeight: 500,
          display: "inline-flex",
          alignItems: "center",
          gap: ".25rem",
        }}
        onClick={() => onSort(sortKey)}
      >
        {label}
        {active ? (dir === "asc" ? <ArrowUp size={12} /> : <ArrowDown size={12} />) : null}
      </button>
    </th>
  );
}

function TablesSection({ databaseId, onOpen }: { databaseId: string; onOpen: (table: string) => void }) {
  const { data, isLoading, error } = useTables(databaseId);
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<TableSortKey>("name");
  const [sortDir, setSortDir] = useState<SortDir>("asc");

  const filtered = useMemo(() => {
    const items = data?.items ?? [];
    const q = search.trim().toLowerCase();
    if (!q) return items;
    return items.filter((t) => t.name.toLowerCase().includes(q));
  }, [data?.items, search]);

  const sorted = useMemo(() => {
    const items = [...filtered];
    items.sort((a, b) => compareTables(a, b, sortKey, sortDir));
    return items;
  }, [filtered, sortKey, sortDir]);

  const paged = useClientPage(sorted);

  function onSort(key: TableSortKey) {
    if (key === sortKey) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir(key === "name" || key === "engine" ? "asc" : "desc");
    }
    paged.setPage(1);
  }

  if (isLoading) {
    return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Connecting to instance…</div>;
  }
  if (error) {
    return <EmptyState title="Could not reach the instance" hint={(error as ApiError).message} />;
  }
  if (!data || data.items.length === 0) {
    return <EmptyState title="No tables" hint="This database has no base tables yet." />;
  }

  return (
    <div>
      <div className="flex items-center gap-2" style={{ marginBottom: ".9rem", maxWidth: "22rem" }}>
        <div style={{ position: "relative", width: "100%" }}>
          <span style={{ position: "absolute", left: 10, top: 9, color: "var(--muted)" }}>
            <Search size={16} />
          </span>
          <input
            className="input"
            style={{ paddingLeft: "2rem" }}
            placeholder="Search tables…"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              paged.setPage(1);
            }}
          />
        </div>
      </div>
      {filtered.length === 0 ? (
        <EmptyState title="No tables match your search" hint="Try a different name or clear the search." />
      ) : (
      <>
      <div className="card" style={{ overflow: "hidden" }}>
      <table className="table">
        <thead>
          <tr>
            <SortableHeader label="Table" sortKey="name" activeKey={sortKey} dir={sortDir} onSort={onSort} />
            <SortableHeader label="Engine" sortKey="engine" activeKey={sortKey} dir={sortDir} onSort={onSort} />
            <SortableHeader label="Rows (est.)" sortKey="row_count" activeKey={sortKey} dir={sortDir} onSort={onSort} />
            <SortableHeader label="Data" sortKey="data_bytes" activeKey={sortKey} dir={sortDir} onSort={onSort} />
            <SortableHeader label="Indexes" sortKey="index_bytes" activeKey={sortKey} dir={sortDir} onSort={onSort} />
            <th />
          </tr>
        </thead>
        <tbody>
          {paged.items.map((t) => (
            <tr key={t.name}>
              <td className="font-medium flex items-center gap-2"><Table2 size={15} /> {t.name}</td>
              <td className="muted">{t.engine}</td>
              <td className="muted">{t.row_count.toLocaleString()}</td>
              <td className="muted">{formatBytes(t.data_bytes)}</td>
              <td className="muted">{formatBytes(t.index_bytes)}</td>
              <td style={{ textAlign: "right" }}>
                <button className="btn btn-ghost btn-sm" onClick={() => onOpen(t.name)}>
                  Browse <ChevronRight size={15} />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      </div>
      <div className="flex items-center justify-end" style={{ marginTop: ".6rem" }}>
        <Pagination page={paged.page} pageCount={paged.pageCount} onPage={paged.setPage} />
      </div>
      </>
      )}
    </div>
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

      {isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Connecting to instance…</div>
      ) : error ? (
        <EmptyState title="Could not reach the instance" hint={(error as ApiError).message} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState title="No accounts have privileges on this database" hint="Use “Grant access” to give a database user access." />
      ) : (
        <div className="card" style={{ overflow: "hidden" }}>
          <table className="table">
            <thead>
              <tr>
                <th>User</th>
                <th>Host</th>
                <th>Privileges</th>
                {canWrite ? <th style={{ textAlign: "right" }}>Actions</th> : null}
              </tr>
            </thead>
            <tbody>
              {data.items.map((g) => (
                <tr key={`${g.user}@${g.host}`}>
                  <td className="font-medium">{g.user}</td>
                  <td className="muted"><code>{g.host}</code></td>
                  <td>
                    <div className="flex" style={{ flexWrap: "wrap", gap: ".25rem" }}>
                      {g.privileges.map((p) => (
                        <span key={p} className="badge badge-gray" style={{ fontSize: 11 }}>{p}</span>
                      ))}
                    </div>
                  </td>
                  {canWrite ? (
                    <td style={{ textAlign: "right" }}>
                      <button className="btn btn-sm btn-danger" onClick={() => onRevoke(g.user, g.host)} disabled={revoke.isPending}>
                        Revoke all
                      </button>
                    </td>
                  ) : null}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

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
