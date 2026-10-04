"use client";

import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";

import { ExportButton } from "@/components/database/export-button";
import { FilterEditor, ImportCSV, needsValue, RowRange } from "@/components/database/row-tools";
import { SchemaView } from "@/components/database/table-structure";
import { ConfirmModal, EmptyState, ErrorText, Field, Modal, Pagination, Spinner } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { exportTableCSV, useBrowseRows, useRowMutations } from "@/lib/hooks";
import { isTruncated } from "@/lib/data-browser/cells";
import { keyOf } from "@/lib/data-browser/pending";
import type { BrowseResult, RowFilter, RowValues, SortKey } from "@/lib/types";
import { ArrowDown, ArrowUp, Filter, Pencil, Plus, Search, Table2, Trash2, Upload, X } from "lucide-react";

const PAGE_SIZE = 50;

export function DataBrowser({
  databaseId,
  table,
  page,
  canWrite,
  onPage,
  onClose,
  onRenamed,
}: {
  databaseId: string;
  table: string;
  page: number;
  canWrite: boolean;
  onPage: (page: number) => void;
  onClose: () => void;
  /** Called when the table was renamed through the structure editor. */
  onRenamed?: (newName: string) => void;
}) {
  const [view, setView] = useState<"data" | "schema">("data");
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [filters, setFilters] = useState<RowFilter[]>([]);
  const [draftFilters, setDraftFilters] = useState<RowFilter[]>([]);
  const [showFilters, setShowFilters] = useState(false);
  const [sort, setSort] = useState<SortKey[]>([]);
  const [editing, setEditing] = useState<{ mode: "insert" } | { mode: "edit"; row: (string | null)[] } | null>(null);
  const [deleting, setDeleting] = useState<(string | null)[] | null>(null);
  const [importOpen, setImportOpen] = useState(false);

  // Debounce the search box. onPage is read through a ref: the parent passes
  // a fresh closure every render, which must not restart the timer.
  const onPageRef = useRef(onPage);
  useEffect(() => {
    onPageRef.current = onPage;
  });
  useEffect(() => {
    if (searchInput === search) return;
    const t = setTimeout(() => {
      setSearch(searchInput);
      onPageRef.current(1);
    }, 350);
    return () => clearTimeout(t);
  }, [searchInput, search]);

  const offset = (page - 1) * PAGE_SIZE;
  const req = useMemo(
    () => ({ filters, sort, search, limit: PAGE_SIZE, offset }),
    [filters, sort, search, offset],
  );
  const { data, isLoading, error, isFetching } = useBrowseRows(databaseId, table, req);
  const editable = canWrite && (data?.key.length ?? 0) > 0;

  const hasFullPage = (data?.rows.length ?? 0) === PAGE_SIZE;
  const knownPages = data && data.total > 0 ? Math.ceil(data.total / PAGE_SIZE) : 0;
  const pageCount = Math.max(knownPages, hasFullPage ? page + 1 : page);

  function toggleSort(column: string) {
    const cur = sort.find((s) => s.column === column);
    setSort(!cur ? [{ column, desc: false }] : !cur.desc ? [{ column, desc: true }] : []);
    onPage(1);
  }

  function applyFilters(e?: FormEvent) {
    e?.preventDefault();
    setFilters(draftFilters.filter((f) => f.column && (!needsValue(f.op) || (f.value ?? "") !== "")));
    onPage(1);
  }

  return (
    <div>
      <div className="flex justify-between items-center" style={{ marginBottom: ".6rem", flexWrap: "wrap", gap: ".5rem" }}>
        <div className="flex items-center gap-2">
          <button className="btn btn-ghost btn-sm" onClick={onClose} aria-label="Back to tables">
            <X size={15} />
          </button>
          <span className="flex items-center gap-2 font-semibold">
            <Table2 size={15} /> {table}
          </span>
          {view === "data" && data ? <RowRange data={data} offset={offset} /> : null}
          {isFetching && view === "data" ? <Spinner /> : null}
        </div>
        <div className="flex items-center gap-2" style={{ flexWrap: "wrap" }}>
          <div className="flex items-center gap-1">
            <button className={`btn btn-sm${view === "data" ? " btn-primary" : ""}`} onClick={() => setView("data")}>
              Data
            </button>
            <button className={`btn btn-sm${view === "schema" ? " btn-primary" : ""}`} onClick={() => setView("schema")}>
              Structure
            </button>
          </div>
          <ExportButton label="Export CSV" run={() => exportTableCSV(databaseId, table)} />
          {canWrite ? (
            <button className="btn btn-sm" onClick={() => setImportOpen(true)}>
              <Upload size={15} /> Import CSV
            </button>
          ) : null}
          {editable && view === "data" ? (
            <button className="btn btn-sm btn-primary" onClick={() => setEditing({ mode: "insert" })}>
              <Plus size={15} /> Add row
            </button>
          ) : null}
        </div>
      </div>

      {view === "schema" ? (
        <SchemaView
          databaseId={databaseId}
          table={table}
          canWrite={canWrite}
          onTableGone={(renamed) => (renamed ? onRenamed?.(renamed) : onClose())}
        />
      ) : (
        <>
          <div className="flex items-center gap-2" style={{ marginBottom: ".6rem", flexWrap: "wrap" }}>
            <div className="flex items-center gap-2" style={{ flex: "1 1 260px" }}>
              <Search size={15} className="muted" />
              <input
                className="input"
                placeholder="Search all columns…"
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                aria-label="Search rows"
              />
            </div>
            <button
              className={`btn btn-sm${filters.length ? " btn-primary" : ""}`}
              onClick={() => {
                setDraftFilters(filters.length ? filters : [{ column: data?.columns[0]?.name ?? "", op: "eq", value: "" }]);
                setShowFilters((v) => !v);
              }}
            >
              <Filter size={15} /> Filters{filters.length ? ` (${filters.length})` : ""}
            </button>
            {filters.length || search || sort.length ? (
              <button
                className="btn btn-ghost btn-sm"
                onClick={() => {
                  setFilters([]);
                  setDraftFilters([]);
                  setSort([]);
                  setSearchInput("");
                  setSearch("");
                  onPage(1);
                }}
              >
                Reset
              </button>
            ) : null}
          </div>

          {showFilters && data ? (
            <FilterEditor
              columns={data.columns}
              filters={draftFilters}
              onChange={setDraftFilters}
              onApply={applyFilters}
            />
          ) : null}

          {data && canWrite && data.key.length === 0 ? (
            <p className="text-sm muted" style={{ marginBottom: ".5rem" }}>
              This table has no primary key or NOT NULL unique key, so rows can&apos;t be edited one by one here. Use the
              SQL console.
            </p>
          ) : null}

          {isLoading ? (
            <div className="flex items-center gap-2 text-sm muted">
              <Spinner /> Loading rows…
            </div>
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
                      {data.columns.map((c) => {
                        const s = sort.find((x) => x.column === c.name);
                        return (
                          <th key={c.name}>
                            <button
                              className="flex items-center gap-1"
                              style={{ all: "unset", cursor: "pointer", display: "inline-flex", gap: 4 }}
                              onClick={() => toggleSort(c.name)}
                              title={`${c.type}${data.key.includes(c.name) ? " · key" : ""} — click to sort`}
                            >
                              {c.name}
                              {data.key.includes(c.name) ? <span className="muted">🔑</span> : null}
                              {s ? s.desc ? <ArrowDown size={12} /> : <ArrowUp size={12} /> : null}
                            </button>
                          </th>
                        );
                      })}
                      {editable ? <th /> : null}
                    </tr>
                  </thead>
                  <tbody>
                    {data.rows.length === 0 ? (
                      <tr>
                        <td colSpan={data.columns.length + (editable ? 1 : 0)} className="muted">
                          {filters.length || search ? "No rows match." : "No rows on this page."}
                        </td>
                      </tr>
                    ) : (
                      data.rows.map((row, ri) => (
                        <tr key={ri}>
                          {row.map((cell, ci) => (
                            <td
                              key={ci}
                              className={cell === null ? "muted" : undefined}
                              style={{ maxWidth: 320, overflow: "hidden", textOverflow: "ellipsis" }}
                              title={cell ?? undefined}
                              onDoubleClick={editable ? () => setEditing({ mode: "edit", row }) : undefined}
                            >
                              {cell === null ? "NULL" : cell}
                            </td>
                          ))}
                          {editable ? (
                            <td style={{ textAlign: "right" }}>
                              <button className="btn btn-ghost btn-sm" aria-label="Edit row" onClick={() => setEditing({ mode: "edit", row })}>
                                <Pencil size={14} />
                              </button>
                              <button className="btn btn-ghost btn-sm" aria-label="Delete row" onClick={() => setDeleting(row)}>
                                <Trash2 size={14} />
                              </button>
                            </td>
                          ) : null}
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
              <div className="flex justify-end items-center" style={{ marginTop: ".6rem" }}>
                <Pagination page={page} pageCount={pageCount} hasMore={hasFullPage} onPage={onPage} />
              </div>
            </>
          )}
        </>
      )}

      {data && editing ? (
        <RowEditor
          databaseId={databaseId}
          table={table}
          data={data}
          row={editing.mode === "edit" ? editing.row : null}
          onClose={() => setEditing(null)}
        />
      ) : null}
      {data && deleting ? (
        <DeleteRow databaseId={databaseId} table={table} data={data} row={deleting} onClose={() => setDeleting(null)} />
      ) : null}
      {importOpen ? <ImportCSV databaseId={databaseId} table={table} onClose={() => setImportOpen(false)} /> : null}
    </div>
  );
}


// locked: the value was truncated for display (over 4 KB), so editing it here
// would write the truncated text back. It can still be set to NULL or changed
// through the SQL console.
type FieldState = { value: string; isNull: boolean; useDefault: boolean; locked: boolean };


function RowEditor({
  databaseId,
  table,
  data,
  row,
  onClose,
}: {
  databaseId: string;
  table: string;
  data: BrowseResult;
  row: (string | null)[] | null;
  onClose: () => void;
}) {
  const { insert, update } = useRowMutations(databaseId, table);
  const isInsert = row === null;
  const [fields, setFields] = useState<FieldState[]>(() =>
    data.columns.map((c, i) => {
      if (isInsert) return { value: "", isNull: false, useDefault: c.has_default, locked: false };
      const v = row[i];
      return { value: v ?? "", isNull: v === null, useDefault: false, locked: isTruncated(v) };
    }),
  );
  const [error, setError] = useState<string | null>(null);
  const busy = insert.isPending || update.isPending;
  const truncated = !isInsert && row.some(isTruncated);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const values: RowValues = {};
    data.columns.forEach((c, i) => {
      const f = fields[i];
      if (isInsert) {
        if (f.useDefault) return;
        values[c.name] = f.isNull ? null : f.value;
        return;
      }
      // Update: send only what changed, so untouched long values are never
      // rewritten from their truncated display form.
      const before = row[i];
      const after = f.isNull ? null : f.value;
      if (after !== before) values[c.name] = after;
    });
    if (!isInsert && Object.keys(values).length === 0) {
      onClose();
      return;
    }
    try {
      if (isInsert) await insert.mutateAsync(values);
      else await update.mutateAsync({ key: keyOf(data.columns, data.key, row), values });
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Save failed");
    }
  }

  return (
    <Modal open onClose={onClose} title={isInsert ? `Add row to ${table}` : `Edit row in ${table}`}>
      <form onSubmit={onSubmit}>
        {truncated ? (
          <p className="text-sm muted" style={{ marginTop: 0 }}>
            Some values are too long to edit here and are locked; use the SQL console to change them.
          </p>
        ) : null}
        <div style={{ maxHeight: "60vh", overflowY: "auto", paddingRight: ".3rem" }}>
          {data.columns.map((c, i) => {
            const f = fields[i];
            const set = (patch: Partial<FieldState>) =>
              setFields((prev) => prev.map((x, j) => (j === i ? { ...x, ...patch } : x)));
            const isKey = data.key.includes(c.name);
            return (
              <Field key={c.name} label={`${c.name}  ·  ${c.type}${isKey ? "  ·  key" : ""}`}>
                <input
                  className="input"
                  value={f.isNull || f.useDefault ? "" : f.value}
                  placeholder={f.useDefault ? "DEFAULT" : f.isNull ? "NULL" : ""}
                  disabled={f.isNull || f.useDefault || f.locked}
                  onChange={(e) => set({ value: e.target.value })}
                />
                <div className="flex items-center gap-3 text-sm" style={{ marginTop: ".25rem" }}>
                  {c.nullable ? (
                    <label className="flex items-center gap-1">
                      <input type="checkbox" checked={f.isNull} onChange={(e) => set({ isNull: e.target.checked, useDefault: false })} />
                      NULL
                    </label>
                  ) : null}
                  {isInsert && c.has_default ? (
                    <label className="flex items-center gap-1">
                      <input type="checkbox" checked={f.useDefault} onChange={(e) => set({ useDefault: e.target.checked, isNull: false })} />
                      default
                    </label>
                  ) : null}
                </div>
              </Field>
            );
          })}
        </div>
        <ErrorText message={error ?? undefined} />
        <div className="flex gap-2" style={{ marginTop: ".8rem" }}>
          <button className="btn btn-primary" type="submit" disabled={busy}>
            {busy ? "Saving…" : isInsert ? "Insert row" : "Save changes"}
          </button>
          <button className="btn" type="button" onClick={onClose} disabled={busy}>
            Cancel
          </button>
        </div>
      </form>
    </Modal>
  );
}

function DeleteRow({
  databaseId,
  table,
  data,
  row,
  onClose,
}: {
  databaseId: string;
  table: string;
  data: BrowseResult;
  row: (string | null)[];
  onClose: () => void;
}) {
  const { remove } = useRowMutations(databaseId, table);
  const [error, setError] = useState<string | null>(null);
  const key = keyOf(data.columns, data.key, row);
  return (
    <ConfirmModal
      open
      danger
      title="Delete row?"
      confirmLabel="Delete row"
      busy={remove.isPending}
      message={
        <>
          <p style={{ marginTop: 0 }}>This permanently deletes one row from {table}.</p>
          <p className="muted">
            {Object.entries(key)
              .map(([k, v]) => `${k} = ${v}`)
              .join(", ")}
          </p>
          <ErrorText message={error ?? undefined} />
        </>
      }
      onConfirm={async () => {
        setError(null);
        try {
          await remove.mutateAsync(key);
          onClose();
        } catch (err) {
          setError(err instanceof ApiError ? err.message : "Delete failed");
        }
      }}
      onCancel={onClose}
    />
  );
}

