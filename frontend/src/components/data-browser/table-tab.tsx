"use client";

import { useCallback, useEffect, useMemo, useReducer, useRef, useState, type Dispatch } from "react";

import { useConfirm } from "@/components/confirm";
import { FilterEditor, ImportCSV, needsValue, RowRange } from "./row-tools";
import { SchemaView } from "@/components/database/table-structure";
import { useToast } from "@/components/toast";
import { EmptyState, Menu, Modal, Pagination, Spinner } from "@/components/ui";
import { cellKind, isTruncated, mutationSQL, prettyValue, quoteTable, type Dialect } from "@/lib/data-browser/cells";
import {
  changeCount,
  emptyPending,
  keyOf,
  pendingReducer,
  rowIdOf,
  toMutations,
} from "@/lib/data-browser/pending";
import { load, save } from "@/lib/data-browser/storage";
import { PAGE_SIZES, splitName, type Action, type OpenRequest, type Tab, type TableView } from "@/lib/data-browser/tabs";
import { exportTableCSV, useApplyRowChanges, useBrowseRows, useForeignKeys } from "@/lib/hooks";
import type { ForeignKey, RowFilter, SortKey } from "@/lib/types";
import {
  ClipboardCopy,
  Download,
  Filter,
  Lock,
  PanelRight,
  Plus,
  RefreshCw,
  Search,
  SquareTerminal,
  Upload,
  X,
} from "lucide-react";

import { cellState, DataGrid, type Cell, type GridRow } from "./data-grid";

const OP_LABEL: Record<string, string> = {
  eq: "=",
  ne: "≠",
  lt: "<",
  lte: "≤",
  gt: ">",
  gte: "≥",
  contains: "contains",
  starts_with: "starts with",
  is_null: "is NULL",
  not_null: "is not NULL",
};

/**
 * TableTab is one open table or view: its rows in an editable grid with
 * search, filters, sort and paging, or its structure. Edits stay pending in
 * the tab until saved.
 */
export function TableTab({
  databaseId,
  tab,
  active,
  canWrite,
  dialect,
  dispatch,
  onDirty,
  onOpen,
  onNewQuery,
}: {
  databaseId: string;
  tab: Tab;
  active: boolean;
  canWrite: boolean;
  dialect: Dialect;
  dispatch: Dispatch<Action>;
  onDirty: (id: string, changes: number) => void;
  onOpen: (req: OpenRequest) => void;
  onNewQuery: (sql?: string) => void;
}) {
  const view = tab.view;
  const isView = tab.kind === "view";
  const { push } = useToast();
  const confirm = useConfirm();
  const setView = useCallback(
    (patch: Partial<TableView>, pin = true) => dispatch({ type: "update", id: tab.id, view: patch, pin }),
    [dispatch, tab.id],
  );

  // ---- search (debounced into the tab state) ----
  const [searchInput, setSearchInput] = useState(view.search);
  // What this box last wrote; any other value arrived from outside (a
  // foreign-key jump resets the search) and replaces the box's text.
  const sentSearch = useRef(view.search);
  useEffect(() => {
    if (view.search !== sentSearch.current) {
      sentSearch.current = view.search;
      setSearchInput(view.search);
    }
  }, [view.search]);
  useEffect(() => {
    if (searchInput === sentSearch.current) return;
    const t = setTimeout(() => {
      sentSearch.current = searchInput;
      setView({ search: searchInput, page: 1 });
    }, 300);
    return () => clearTimeout(t);
  }, [searchInput, setView]);

  const req = useMemo(
    () => ({
      filters: view.filters,
      sort: view.sort,
      search: view.search,
      limit: view.pageSize,
      offset: (view.page - 1) * view.pageSize,
    }),
    [view.filters, view.sort, view.search, view.pageSize, view.page],
  );
  const { data, isLoading, error, isFetching, refetch } = useBrowseRows(databaseId, tab.name, req);
  const fkQuery = useForeignKeys(databaseId, tab.kind === "table" ? tab.name : "");
  const fks = useMemo(() => {
    const out: Record<string, ForeignKey> = {};
    for (const fk of fkQuery.data ?? []) if (fk.columns.length === 1 && fk.ref_columns.length === 1) out[fk.columns[0]] = fk;
    return out;
  }, [fkQuery.data]);

  const { schema } = splitName(tab.name, dialect === "postgres");
  const quoted = quoteTable(tab.name, dialect, schema);

  // ---- pending edits ----
  const [pending, pd] = useReducer(pendingReducer, emptyPending);
  const count = changeCount(pending);
  useEffect(() => onDirty(tab.id, count), [onDirty, tab.id, count]);
  useEffect(() => () => onDirty(tab.id, 0), [onDirty, tab.id]);
  const [saving, setSaving] = useState(false);
  const [review, setReview] = useState(false);
  const applyChanges = useApplyRowChanges(databaseId, tab.name);

  const columns = useMemo(() => data?.columns ?? [], [data]);
  const key = useMemo(() => data?.key ?? [], [data]);
  const editable = canWrite && !isView && key.length > 0;

  const rows = useMemo<GridRow[]>(() => {
    const out: GridRow[] = pending.inserts.map((r) => ({ kind: "new", id: r.id, values: r.values, error: pending.errors[r.id] }));
    (data?.rows ?? []).forEach((cells, index) => {
      const rowId = key.length ? rowIdOf(key, keyOf(columns, key, cells)) : null;
      out.push({
        kind: "data",
        index,
        cells,
        rowId,
        deleted: rowId !== null && rowId in pending.deletes,
        changes: rowId !== null ? pending.updates[rowId]?.values : undefined,
        error: rowId !== null ? pending.errors[rowId] : undefined,
      });
    });
    return out;
  }, [data, pending, columns, key]);

  // ---- selection ----
  const [cursor, setCursor] = useState<Cell | null>(null);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [editRequest, setEditRequest] = useState<Cell | null>(null);
  const clearEditRequest = useCallback(() => setEditRequest(null), []);
  const rowsRef = useRef(rows);
  useEffect(() => {
    // A new page or refetch invalidates row positions.
    if (rowsRef.current.length !== rows.length) {
      setSelected(new Set());
      setCursor((c) => (c && c.row < rows.length ? c : null));
    }
    rowsRef.current = rows;
  }, [rows]);
  useEffect(() => {
    setSelected(new Set());
    setCursor(null);
  }, [view.page, view.pageSize]);

  const pin = useCallback(() => {
    if (tab.preview) dispatch({ type: "pin", id: tab.id });
  }, [dispatch, tab.id, tab.preview]);

  function onEdit(r: number, c: number, value: string | null | undefined) {
    const row = rows[r];
    const col = columns[c];
    if (!row || !col) return;
    pin();
    if (row.kind === "new") {
      pd({ type: "setNew", id: row.id, column: col.name, value });
    } else if (row.rowId !== null && value !== undefined) {
      pd({ type: "set", rowId: row.rowId, key: keyOf(columns, key, row.cells), column: col.name, value, original: row.cells[c] });
    }
  }

  function onDeleteRows(idx: number[]) {
    pin();
    const del: { rowId: string; key: Record<string, string | null> }[] = [];
    for (const i of idx) {
      const row = rows[i];
      if (row?.kind === "new") pd({ type: "removeNew", id: row.id });
      else if (row?.kind === "data" && row.rowId !== null) del.push({ rowId: row.rowId, key: keyOf(columns, key, row.cells) });
    }
    if (del.length) pd({ type: "delete", rows: del });
    setSelected(new Set());
  }

  function addRow() {
    pin();
    pd({ type: "insert", columns });
    setSelected(new Set());
    // Start typing straight away in the first column that needs a value.
    setEditRequest({ row: 0, col: Math.max(0, columns.findIndex((c) => !c.has_default)) });
    if (view.mode !== "data") setView({ mode: "data" });
  }

  const addFilter = (f: RowFilter) => setView({ filters: [...view.filters, f], page: 1 });

  function onSort(column: string, dir: "asc" | "desc" | "toggle" | "clear", additive: boolean) {
    const cur = view.sort.find((s) => s.column === column);
    let next: SortKey[];
    if (dir === "clear") next = view.sort.filter((s) => s.column !== column);
    else if (dir === "asc" || dir === "desc") next = [{ column, desc: dir === "desc" }];
    else {
      // asc → desc → off
      const nextKey: SortKey | null = !cur ? { column, desc: false } : !cur.desc ? { column, desc: true } : null;
      if (additive) {
        next = cur
          ? view.sort.flatMap((s) => (s.column === column ? (nextKey ? [nextKey] : []) : [s]))
          : [...view.sort, nextKey!];
      } else {
        next = nextKey ? [nextKey] : [];
      }
    }
    setView({ sort: next, page: 1 });
  }

  function follow(fk: ForeignKey, value: string) {
    const { label } = splitName(fk.ref_table, dialect === "postgres");
    onOpen({
      kind: "table",
      name: fk.ref_table,
      label,
      view: { mode: "data", search: "", filters: [{ column: fk.ref_columns[0], op: "eq", value }] },
    });
  }

  const doSave = useCallback(async () => {
    const mutations = toMutations(pending);
    if (mutations.length === 0 || saving) return;
    setSaving(true);
    try {
      const results = await applyChanges(mutations);
      pd({ type: "results", results });
      const failed = results.filter((r) => !r.ok).length;
      if (failed === 0) push("success", `Saved ${results.length} change${results.length === 1 ? "" : "s"} to ${tab.label}`);
      else
        push(
          "error",
          `${failed} of ${results.length} change${results.length === 1 ? "" : "s"} failed${
            failed < results.length ? "; the rest were saved" : ""
          }. Hover the red rows to see why.`,
        );
    } finally {
      setSaving(false);
    }
  }, [pending, saving, applyChanges, push, tab.label]);

  async function discard() {
    if (count === 0) return;
    const ok = await confirm({
      title: "Discard unsaved changes?",
      message: `${count} change${count === 1 ? "" : "s"} to ${tab.label} will be thrown away.`,
      confirmLabel: "Discard",
      danger: true,
    });
    if (ok) pd({ type: "discard" });
  }

  // Ctrl/⌘+S saves the active tab.
  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === "s") {
        e.preventDefault();
        void doSave();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [active, doSave]);

  // ---- toolbar state ----
  const [showFilters, setShowFilters] = useState(false);
  const [draftFilters, setDraftFilters] = useState<RowFilter[]>([]);
  const [importOpen, setImportOpen] = useState(false);
  const [panel, setPanel] = useState(() => load("value-panel") === "1");
  const togglePanel = () => {
    setPanel((v) => {
      save("value-panel", v ? null : "1");
      return !v;
    });
  };

  const readOnlyReason = isView
    ? "Views are read-only."
    : !canWrite
      ? "You have read-only access to this database."
      : data && key.length === 0
        ? "This table has no primary key or NOT NULL unique key, so rows can't be edited here. Use the SQL console."
        : null;

  const offset = (view.page - 1) * view.pageSize;
  const hasFullPage = (data?.rows.length ?? 0) === view.pageSize;
  const knownPages = data && data.total > 0 ? Math.ceil(data.total / view.pageSize) : 0;
  const pageCount = Math.max(knownPages, hasFullPage ? view.page + 1 : view.page);
  const filtered = view.filters.length > 0 || view.search !== "";

  const counts = {
    edited: Object.keys(pending.updates).length,
    added: pending.inserts.length,
    deleted: Object.keys(pending.deletes).length,
    failed: Object.keys(pending.errors).length,
  };

  return (
    <div className="dbx-pane">
      <div className="dbx-toolbar">
        <div className="segmented dbx-seg" role="group" aria-label="View">
          <button aria-pressed={view.mode === "data"} onClick={() => setView({ mode: "data" }, false)}>
            Data
          </button>
          <button aria-pressed={view.mode === "structure"} onClick={() => setView({ mode: "structure" }, false)}>
            Structure
          </button>
        </div>
        {view.mode === "data" ? (
          <>
            <div className="dbx-search dbx-toolbar-search">
              <Search size={14} />
              <input
                className="input"
                placeholder="Search all columns…"
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Escape" && searchInput) {
                    e.stopPropagation();
                    setSearchInput("");
                  }
                }}
                aria-label="Search rows"
              />
              {searchInput ? (
                <button className="dbx-search-clear" onClick={() => setSearchInput("")} aria-label="Clear search">
                  <X size={13} />
                </button>
              ) : null}
            </div>
            <button
              className={`btn btn-sm${view.filters.length ? " btn-active" : ""}`}
              aria-expanded={showFilters}
              onClick={() => {
                setDraftFilters(view.filters.length ? view.filters : [{ column: columns[0]?.name ?? "", op: "eq", value: "" }]);
                setShowFilters((v) => !v);
              }}
              disabled={columns.length === 0}
            >
              <Filter size={14} /> Filter{view.filters.length ? ` · ${view.filters.length}` : ""}
            </button>
          </>
        ) : null}
        <div style={{ flex: 1 }} />
        {readOnlyReason ? (
          <span className="badge dbx-readonly" title={readOnlyReason}>
            <Lock size={11} /> Read-only
          </span>
        ) : null}
        {isFetching && !isLoading ? <Spinner label="Refreshing" /> : null}
        {view.mode === "data" ? (
          <>
            <button className="btn btn-ghost btn-icon" onClick={() => void refetch()} title="Refresh rows" aria-label="Refresh rows">
              <RefreshCw size={14} />
            </button>
            <button
              className={`btn btn-ghost btn-icon${panel ? " btn-active" : ""}`}
              onClick={togglePanel}
              title="Value panel: see and edit the selected cell in full"
              aria-label="Toggle value panel"
              aria-pressed={panel}
            >
              <PanelRight size={15} />
            </button>
            {editable ? (
              <button className="btn btn-sm btn-primary" onClick={addRow}>
                <Plus size={14} /> Row
              </button>
            ) : null}
          </>
        ) : null}
        <Menu
          label="More table actions"
          items={[
            {
              label: "New query on this table",
              icon: <SquareTerminal size={15} />,
              onSelect: () => onNewQuery(`SELECT *\nFROM ${quoted}\nLIMIT 100;`),
            },
            {
              label: "Export CSV",
              icon: <Download size={15} />,
              hidden: isView,
              onSelect: () =>
                void exportTableCSV(databaseId, tab.name).catch((err: unknown) =>
                  push("error", err instanceof Error ? err.message : "Export failed"),
                ),
            },
            { label: "Import CSV…", icon: <Upload size={15} />, hidden: !canWrite || isView, onSelect: () => setImportOpen(true) },
            {
              label: "Copy name",
              icon: <ClipboardCopy size={15} />,
              onSelect: () => void navigator.clipboard?.writeText(tab.name),
            },
          ]}
        />
      </div>

      {view.mode === "data" && showFilters && columns.length ? (
        <div className="dbx-filters">
          <FilterEditor
            columns={columns}
            filters={draftFilters}
            onChange={setDraftFilters}
            onApply={(e) => {
              e?.preventDefault();
              setView({
                filters: draftFilters.filter((f) => f.column && (!needsValue(f.op) || (f.value ?? "") !== "")),
                page: 1,
              });
              setShowFilters(false);
            }}
          />
        </div>
      ) : null}

      {view.mode === "data" && view.filters.length && !showFilters ? (
        <div className="dbx-chips">
          {view.filters.map((f, i) => (
            <span key={i} className="dbx-chip">
              <span className="font-medium">{f.column}</span> {OP_LABEL[f.op]}
              {needsValue(f.op) ? <span className="mono"> {f.value}</span> : null}
              <button
                aria-label="Remove filter"
                onClick={() => setView({ filters: view.filters.filter((_, j) => j !== i), page: 1 })}
              >
                <X size={11} />
              </button>
            </span>
          ))}
          <button className="btn btn-ghost btn-sm" onClick={() => setView({ filters: [], page: 1 })}>
            Clear filters
          </button>
        </div>
      ) : null}

      {view.mode === "structure" ? (
        <div className="dbx-structure">
          <SchemaView
            databaseId={databaseId}
            table={tab.name}
            canWrite={canWrite && !isView}
            onTableGone={(renamed) => {
              if (renamed) {
                // Renames keep the schema; the structure editor reports the bare new name.
                const name = schema && !renamed.includes(".") ? `${schema}.${renamed}` : renamed;
                dispatch({ type: "rename", id: tab.id, name, label: splitName(name, dialect === "postgres").label });
              } else {
                dispatch({ type: "close", id: tab.id });
              }
            }}
          />
        </div>
      ) : (
        <div className="dbx-body">
          <div className="dbx-grid-wrap">
            {isLoading ? (
              <div className="dbx-center">
                <Spinner /> Loading rows…
              </div>
            ) : error ? (
              <div className="dbx-center">
                <EmptyState
                  title="Could not load rows"
                  hint={(error as Error).message}
                  action={
                    <button className="btn" onClick={() => void refetch()}>
                      Try again
                    </button>
                  }
                />
              </div>
            ) : (
              <DataGrid
                columns={columns}
                rows={rows}
                keyCols={key}
                offset={offset}
                dialect={dialect}
                quotedTable={quoted}
                sort={view.sort}
                onSort={onSort}
                widths={view.widths}
                onWidths={(widths) => setView({ widths }, false)}
                fks={fks}
                editable={editable}
                cursor={cursor}
                setCursor={setCursor}
                selected={selected}
                setSelected={setSelected}
                onEdit={onEdit}
                onRevertCell={(r, c) => {
                  const row = rows[r];
                  if (row?.kind === "data" && row.rowId) pd({ type: "revertCell", rowId: row.rowId, column: columns[c].name });
                }}
                onDeleteRows={onDeleteRows}
                onRestoreRows={(idx) =>
                  idx.forEach((i) => {
                    const row = rows[i];
                    if (row?.kind === "data" && row.rowId) pd({ type: "undelete", rowId: row.rowId });
                  })
                }
                onDuplicate={(r) => {
                  const row = rows[r];
                  if (!row) return;
                  pin();
                  pd({ type: "duplicate", columns, key, row: columns.map((c, i) => cellState(row, c, i).value) });
                  setEditRequest({ row: 0, col: Math.max(0, columns.findIndex((c) => key.includes(c.name) && !c.has_default)) });
                }}
                onFilterBy={(column, value) =>
                  addFilter(value === null ? { column, op: "is_null" } : { column, op: "eq", value })
                }
                onFollow={follow}
                editRequest={editRequest}
                onEditRequestDone={clearEditRequest}
                emptyText={
                  filtered
                    ? "No rows match the search or filters."
                    : view.page > 1
                      ? "No rows on this page."
                      : isView
                        ? "This view returns no rows."
                        : "This table is empty."
                }
              />
            )}
          </div>
          {panel ? (
            <ValuePanel
              row={cursor ? rows[cursor.row] : undefined}
              col={cursor ? columns[cursor.col] : undefined}
              ci={cursor?.col ?? -1}
              editable={cursor ? rows[cursor.row]?.kind === "new" || (editable && rows[cursor.row]?.kind === "data") : false}
              onApply={(v) => cursor && onEdit(cursor.row, cursor.col, v)}
              onClose={togglePanel}
            />
          ) : null}
        </div>
      )}

      {view.mode === "data" && (count > 0 || saving) ? (
        <div className={`dbx-commit${counts.failed ? " has-errors" : ""}`} role="status">
          <span className="dbx-commit-dot" />
          <span className="font-medium">
            {count} unsaved change{count === 1 ? "" : "s"}
          </span>
          <span className="muted text-sm hide-sm">
            {[
              counts.edited && `${counts.edited} edited`,
              counts.added && `${counts.added} new`,
              counts.deleted && `${counts.deleted} to delete`,
            ]
              .filter(Boolean)
              .join(" · ")}
            {counts.failed ? <span style={{ color: "var(--danger)" }}> · {counts.failed} failed</span> : null}
          </span>
          {counts.failed ? (
            <span className="dbx-commit-error" title={Object.values(pending.errors).join("\n")}>
              {Object.values(pending.errors)[0]}
            </span>
          ) : null}
          <div style={{ flex: 1 }} />
          <button className="btn btn-ghost btn-sm" onClick={() => setReview(true)} disabled={saving}>
            Review SQL
          </button>
          <button className="btn btn-sm" onClick={() => void discard()} disabled={saving}>
            Discard
          </button>
          <button className="btn btn-sm btn-primary" onClick={() => void doSave()} disabled={saving}>
            {saving ? <Spinner label="Saving" /> : null} Save <kbd className="dbx-kbd-primary hide-sm">Ctrl S</kbd>
          </button>
        </div>
      ) : null}

      {view.mode === "data" ? (
        <div className="dbx-footer">
          <span className="text-sm muted">
            {data && data.rows.length > 0 ? <RowRange data={data} offset={offset} /> : data ? "No rows" : ""}
            {selected.size > 1 ? ` · ${selected.size} selected` : ""}
          </span>
          <div style={{ flex: 1 }} />
          <label className="flex items-center gap-1 text-sm muted">
            <span className="hide-sm">Rows per page</span>
            <select
              className="input dbx-pagesize"
              value={view.pageSize}
              onChange={(e) => setView({ pageSize: Number(e.target.value), page: 1 })}
              aria-label="Rows per page"
            >
              {PAGE_SIZES.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          </label>
          <Pagination
            page={view.page}
            pageCount={pageCount}
            hasMore={hasFullPage}
            onPage={(page) => setView({ page })}
          />
        </div>
      ) : null}

      {review ? (
        <Modal open wide onClose={() => setReview(false)} title={`Changes to ${tab.label}`}>
          <p className="text-sm muted" style={{ marginTop: 0 }}>
            What saving will do, as SQL. Each row is applied on its own, in this order; if one fails, the others still
            go through and the failed one stays here for you to fix.
          </p>
          <pre className="dbx-review">
            {toMutations(pending)
              .map((m) => mutationSQL(m, quoted, columns, dialect))
              .join("\n")}
          </pre>
          <div className="flex gap-2" style={{ marginTop: ".8rem" }}>
            <button
              className="btn btn-primary"
              onClick={() => {
                setReview(false);
                void doSave();
              }}
            >
              Save {count} change{count === 1 ? "" : "s"}
            </button>
            <button className="btn" onClick={() => setReview(false)}>
              Close
            </button>
          </div>
        </Modal>
      ) : null}
      {importOpen ? <ImportCSV databaseId={databaseId} table={tab.name} onClose={() => setImportOpen(false)} /> : null}
    </div>
  );
}

/** ValuePanel shows the selected cell in full and edits long or multi-line values. */
function ValuePanel({
  row,
  col,
  ci,
  editable,
  onApply,
  onClose,
}: {
  row?: GridRow;
  col?: { name: string; type: string; nullable: boolean; has_default: boolean };
  ci: number;
  editable: boolean;
  onApply: (v: string | null) => void;
  onClose: () => void;
}) {
  const st = row && col ? cellState(row, col, ci) : null;
  const [draft, setDraft] = useState<string | null>(null);
  const cellKey = row && col ? `${row.kind === "new" ? row.id : row.index}:${col.name}` : "";
  const [prevKey, setPrevKey] = useState(cellKey);
  if (prevKey !== cellKey) {
    setPrevKey(cellKey);
    setDraft(null);
  }
  const { push } = useToast();
  const kind = col ? cellKind(col.type) : "text";
  const locked = st ? isTruncated(st.original) && !st.changed : false;

  return (
    <aside className="dbx-valuepanel" aria-label="Value panel">
      <div className="dbx-valuepanel-head">
        <span className="truncate font-medium">{col ? col.name : "Value"}</span>
        {col ? <span className="muted text-xs truncate">{col.type}</span> : null}
        <div style={{ flex: 1 }} />
        <button className="btn btn-ghost btn-icon" onClick={onClose} aria-label="Close value panel">
          <X size={14} />
        </button>
      </div>
      {!st ? (
        <p className="muted text-sm" style={{ padding: ".75rem" }}>
          Select a cell to see its full value here.
        </p>
      ) : draft !== null ? (
        <div className="dbx-valuepanel-body">
          <textarea
            className="input dbx-valuepanel-edit"
            value={draft}
            autoFocus
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
                onApply(draft);
                setDraft(null);
              } else if (e.key === "Escape") {
                e.stopPropagation();
                setDraft(null);
              }
            }}
            spellCheck={false}
          />
          <div className="flex gap-2" style={{ flexWrap: "wrap" }}>
            <button
              className="btn btn-sm btn-primary"
              onClick={() => {
                onApply(draft);
                setDraft(null);
              }}
            >
              Apply
            </button>
            <button className="btn btn-sm" onClick={() => setDraft(null)}>
              Cancel
            </button>
            {kind === "json" ? (
              <button className="btn btn-ghost btn-sm" onClick={() => setDraft(prettyValue(draft, "json"))}>
                Format JSON
              </button>
            ) : null}
          </div>
          <p className="muted text-xs" style={{ margin: 0 }}>
            Ctrl+Enter applies. Nothing is written until you save the tab.
          </p>
        </div>
      ) : (
        <div className="dbx-valuepanel-body">
          {st.isDefault ? (
            <span className="dbx-pill">DEFAULT</span>
          ) : st.value === null ? (
            <span className="dbx-pill">NULL</span>
          ) : (
            <pre className="dbx-valuepanel-pre">{prettyValue(st.value, kind)}</pre>
          )}
          <div className="flex gap-2" style={{ flexWrap: "wrap" }}>
            {st.value !== null ? (
              <button
                className="btn btn-sm"
                onClick={() => void navigator.clipboard?.writeText(st.value ?? "").then(() => push("success", "Copied value"))}
              >
                <ClipboardCopy size={13} /> Copy
              </button>
            ) : null}
            {editable && !locked ? (
              <button className="btn btn-sm" onClick={() => setDraft(st.value ?? "")}>
                Edit
              </button>
            ) : null}
            {editable && col?.nullable && st.value !== null ? (
              <button className="btn btn-ghost btn-sm" onClick={() => onApply(null)}>
                Set NULL
              </button>
            ) : null}
          </div>
          {locked ? (
            <p className="muted text-xs" style={{ margin: 0 }}>
              Shown cut short at 4 KB, so it can&apos;t be edited here. Use the SQL console.
            </p>
          ) : st.changed ? (
            <p className="text-xs" style={{ margin: 0, color: "var(--warning)" }}>
              Changed — was {st.original === null ? "NULL" : `“${st.original.slice(0, 80)}${st.original.length > 80 ? "…" : ""}”`}
            </p>
          ) : null}
        </div>
      )}
    </aside>
  );
}
