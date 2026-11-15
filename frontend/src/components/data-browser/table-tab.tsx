"use client";

import { useCallback, useEffect, useMemo, useReducer, useRef, useState, type Dispatch } from "react";

import { useConfirm } from "@/components/confirm";
import { JsonEditor } from "./json-editor";
import { JsonViewer } from "./json-viewer";
import { FilterEditor, ImportCSV, RowRange } from "./row-tools";
import { SchemaView } from "@/components/database/table-structure";
import { useCopy, useToast } from "@/components/toast";
import {
  Badge,
  Button,
  EmptyState,
  IconButton,
  Input,
  Menu,
  Modal,
  Pagination,
  SegmentedControl,
  Select,
  Spinner,
  Textarea,
} from "@/components/ui";
import {
  cellKind,
  compactJson,
  formatJson,
  isTruncated,
  mutationSQL,
  parseJsonDoc,
  prettyValue,
  quoteTable,
  type Dialect,
} from "@/lib/data-browser/cells";
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
import { fkByColumn, followFilters, referencingFilters, type RowRecord } from "@/lib/data-browser/fk";
import { describeValue, filterFor, fromDraft, opLabel, toDraft } from "@/lib/data-browser/filters";
import { applySavedState } from "@/lib/data-browser/view-state";
import { exportTableCSV, useTableViews, useApplyRowChanges, useBrowseRows, useForeignKeys, useReferencedBy } from "@/lib/hooks";
import type { ForeignKey, QueryResult, RowFilter, SavedTableView, SortKey } from "@/lib/types";
import {
  ClipboardCopy,
  Download,
  Filter,
  Lock,
  History,
  PanelRight,
  ListOrdered,
  Rows2,
  Rows3,
  Plus,
  RefreshCw,
  Search,
  SquareTerminal,
  Upload,
  X,
  ArchiveRestore,
} from "lucide-react";

import { cellState, DataGrid, type Cell, type GridRow } from "./data-grid";
import { CommandPanel } from "./command-panel";
import { ResultGrid } from "./result-grid";
import { BackupCompare } from "./backup-compare";
import { RowHistory, useRowUndo } from "./row-history";
import { ColumnsMenu } from "./columns-menu";
import { ViewsMenu } from "./views-menu";
import { RowPanel } from "./row-panel";
import { GridSkeleton } from "./skeleton";


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
  canBackup,
  dialect,
  engine,
  dispatch,
  onDirty,
  onOpen,
  onNewQuery,
  applySaved,
  onSavedApplied,
}: {
  databaseId: string;
  tab: Tab;
  active: boolean;
  canWrite: boolean;
  canBackup: boolean;
  dialect: Dialect;
  /** Engine of the database, for the command panel's editor. */
  engine?: string;
  dispatch: Dispatch<Action>;
  onDirty: (id: string, changes: number) => void;
  onOpen: (req: OpenRequest) => void;
  onNewQuery: (sql?: string) => void;
  /** A saved view to apply once the table's views load (a ?saved= link). */
  applySaved?: string;
  onSavedApplied?: () => void;
}) {
  const view = tab.view;
  const isView = tab.kind === "view";
  const { push } = useToast();
  const copy = useCopy();
  const confirm = useConfirm();
  const setView = useCallback(
    (patch: Partial<TableView>, pin = true) => dispatch({ type: "update", id: tab.id, view: patch, pin }),
    [dispatch, tab.id],
  );

  // ---- saved views ----
  const savedViews = useTableViews(databaseId, tab.name, Boolean(applySaved));
  const applyView = useCallback(
    (v: SavedTableView) => setView(applySavedState(v.state, v.id)),
    [setView],
  );
  useEffect(() => {
    if (!applySaved || !savedViews.isFetched) return;
    const v = savedViews.data?.find((x) => x.id === applySaved);
    if (v) applyView(v);
    else push("error", "That saved view is gone, or it is not shared with you. Showing the whole table.");
    onSavedApplied?.();
  }, [applySaved, savedViews.isFetched, savedViews.data, applyView, push, onSavedApplied]);

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
  const fks = useMemo(() => fkByColumn(fkQuery.data ?? []), [fkQuery.data]);
  const incomingQuery = useReferencedBy(databaseId, tab.kind === "table" ? tab.name : "");
  const incoming = useMemo(() => incomingQuery.data ?? [], [incomingQuery.data]);

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
  const rowUndo = useRowUndo(databaseId);

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

  function openFiltered(table: string, filters: RowFilter[] | null) {
    if (!filters) return;
    const { label } = splitName(table, dialect === "postgres");
    onOpen({ kind: "table", name: table, label, view: { mode: "data", search: "", filters } });
  }
  const follow = (fk: ForeignKey, row: RowRecord) => openFiltered(fk.ref_table, followFilters(fk, row));
  const referencing = (fk: ForeignKey, row: RowRecord) => openFiltered(fk.table ?? "", referencingFilters(fk, row));

  const doSave = useCallback(async () => {
    const mutations = toMutations(pending);
    if (mutations.length === 0 || saving) return;
    setSaving(true);
    try {
      const { results, batchId } = await applyChanges(mutations);
      pd({ type: "results", results });
      const failed = results.filter((r) => !r.ok).length;
      const undoable = results.some((r) => r.ok && r.undoable);
      if (failed === 0)
        push(
          "success",
          `Saved ${results.length} change${results.length === 1 ? "" : "s"} to ${tab.label}`,
          undoable ? { label: "Undo", onClick: () => void rowUndo.run({ batchId }) } : undefined,
        );
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
  }, [pending, saving, applyChanges, push, tab.label, rowUndo]);

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

  // Alt+PageDown / Alt+PageUp turn the page of the active tab.
  const pageRef = useRef({ page: view.page, more: false });
  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      if (!e.altKey || e.ctrlKey || e.metaKey || (e.key !== "PageDown" && e.key !== "PageUp")) return;
      if (document.querySelector('[role="dialog"]')) return;
      const { page, more } = pageRef.current;
      if (e.key === "PageDown" && more) setView({ page: page + 1 });
      else if (e.key === "PageUp" && page > 1) setView({ page: page - 1 });
      else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [active, setView]);

  // ---- toolbar state ----
  const [showFilters, setShowFilters] = useState(false);
  const [draftFilters, setDraftFilters] = useState<RowFilter[]>([]);
  const [filterFocus, setFilterFocus] = useState<number | undefined>(undefined);
  /** openFilters shows the editor on the current filters, plus one to add, focused. */
  const openFilters = (focus?: number, add?: RowFilter) => {
    const draft = view.filters.map(toDraft);
    if (add) draft.push(add);
    if (draft.length === 0) draft.push({ column: columns[0]?.name ?? "", op: "eq", value: "" });
    setDraftFilters(draft);
    setFilterFocus(add ? draft.length - 1 : focus);
    setShowFilters(true);
  };
  const [importOpen, setImportOpen] = useState(false);
  // The right-hand panel: one cell in full, or the whole row as a form.
  const [side, setSide] = useState<"none" | "value" | "row">(() => {
    const v = load("value-panel");
    return v === "1" ? "value" : v === "row" ? "row" : "none";
  });
  const panel = side === "value";
  const showSide = (next: "value" | "row") =>
    setSide((cur) => {
      const v = cur === next ? "none" : next;
      save("value-panel", v === "none" ? null : v === "value" ? "1" : "row");
      return v;
    });
  // Editing a JSON cell opens the value panel in edit mode. Each request has
  // its own id; the panel starts editing once per id.
  const [panelEdit, setPanelEdit] = useState<number | null>(null);
  const editJsonInPanel = () => {
    setPanelEdit((n) => (n ?? 0) + 1);
    if (side !== "value") showSide("value");
  };
  const togglePanel = () => {
    setPanelEdit(null);
    showSide("value");
  };
  const toggleRowPanel = () => showSide("row");
  const openRowPanel = () => {
    if (side !== "row") showSide("row");
  };
  const [density, setDensity] = useState<"compact" | "comfortable">(() =>
    load("density") === "comfortable" ? "comfortable" : "compact",
  );
  const toggleDensity = () =>
    setDensity((d) => {
      const next = d === "compact" ? "comfortable" : "compact";
      save("density", next === "compact" ? null : next);
      return next;
    });
  const [rowNumbers, setRowNumbers] = useState(() => load("row-numbers") !== "0");
  const toggleRowNumbers = () =>
    setRowNumbers((v) => {
      save("row-numbers", v ? "0" : null);
      return !v;
    });
  const [historyOpen, setHistoryOpen] = useState(false);
  const [compareOpen, setCompareOpen] = useState(false);
  // The SQL command panel under the grid; its open state is remembered.
  const [commandOpen, setCommandOpen] = useState(() => load("command-panel") === "1");
  // A command panel read's rows, shown in place of the table until dismissed.
  const [commandResult, setCommandResult] = useState<{ sql: string; result: QueryResult } | null>(null);
  const toggleCommand = () =>
    setCommandOpen((v) => {
      if (v) setCommandResult(null);
      save("command-panel", v ? null : "1");
      return !v;
    });

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
  useEffect(() => {
    pageRef.current = { page: view.page, more: view.page < pageCount };
  });
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
        <SegmentedControl
          className="dbx-seg"
          label="View"
          value={view.mode}
          onChange={(mode) => setView({ mode }, false)}
          options={[
            { value: "data", label: "Data" },
            { value: "structure", label: "Structure" },
          ]}
        />
        {view.mode === "data" ? (
          <>
            <div className="dbx-search dbx-toolbar-search">
              <Search size={14} aria-hidden />
              <Input
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
            <Button
              size="sm"
              active={view.filters.length > 0}
              icon={<Filter size={14} aria-hidden />}
              aria-expanded={showFilters}
              onClick={() => (showFilters ? setShowFilters(false) : openFilters())}
              disabled={columns.length === 0}
            >
              Filter{view.filters.length ? ` · ${view.filters.length}` : ""}
            </Button>
            <ViewsMenu
              databaseId={databaseId}
              table={tab.name}
              view={view}
              onApply={applyView}
              onClearSaved={() => setView({ savedView: undefined }, false)}
            />
            <ColumnsMenu
              columns={columns}
              hidden={view.hidden ?? []}
              onChange={(hidden) => setView({ hidden, frozen: Math.min(view.frozen ?? 0, columns.length - hidden.length - 1) }, false)}
            />
          </>
        ) : null}
        <div className="flex-1" />
        {readOnlyReason ? (
          <Badge className="dbx-readonly" title={readOnlyReason}>
            <Lock size={11} aria-hidden /> Read-only
          </Badge>
        ) : null}
        {isFetching && !isLoading ? <Spinner label="Refreshing" /> : null}
        {view.mode === "data" ? (
          <>
            <IconButton label="Refresh rows" onClick={() => void refetch()}>
              <RefreshCw size={14} aria-hidden />
            </IconButton>
            {/* noTooltip + title: the native title says more than the accessible name. */}
            {!isView ? (
              <IconButton
                noTooltip
                label="Toggle change history"
                active={historyOpen}
                onClick={() => setHistoryOpen((v) => !v)}
                title="Change history: edits made here, with undo"
                aria-pressed={historyOpen}
              >
                <History size={15} aria-hidden />
              </IconButton>
            ) : null}
            <IconButton
              noTooltip
              label="Toggle value panel"
              active={panel}
              onClick={togglePanel}
              title="Value panel: see and edit the selected cell in full"
              aria-pressed={panel}
            >
              <PanelRight size={15} aria-hidden />
            </IconButton>
            <IconButton
              noTooltip
              label="Toggle row panel"
              active={side === "row"}
              onClick={toggleRowPanel}
              title="Row panel: the selected row as a form (Shift+Enter)"
              aria-pressed={side === "row"}
            >
              <Rows3 size={15} aria-hidden />
            </IconButton>
            <IconButton
              noTooltip
              label="Toggle command panel"
              active={commandOpen}
              onClick={toggleCommand}
              title="Command panel: run SQL under the grid"
              aria-pressed={commandOpen}
            >
              <SquareTerminal size={15} aria-hidden />
            </IconButton>
            {editable ? (
              <Button size="sm" variant="primary" icon={<Plus size={14} aria-hidden />} onClick={addRow}>
                Row
              </Button>
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
              label: filtered ? "Export matching rows (CSV)" : "Export all rows (CSV)",
              icon: <Download size={15} />,
              onSelect: () =>
                void exportTableCSV(databaseId, tab.name, { filters: view.filters, sort: view.sort, search: view.search }).catch(
                  (err: unknown) => push("error", err instanceof Error ? err.message : "Export failed"),
                ),
            },
            { label: "Import CSV…", icon: <Upload size={15} />, hidden: !canWrite || isView, onSelect: () => setImportOpen(true) },
            {
              label: "Compare with a backup…",
              icon: <ArchiveRestore size={15} />,
              hidden: isView,
              onSelect: () => setCompareOpen(true),
            },
            {
              label: "Copy name",
              icon: <ClipboardCopy size={15} />,
              onSelect: () => copy(tab.name, `Copied ${tab.name}`),
            },
            {
              label: density === "compact" ? "Taller rows" : "Compact rows",
              icon: <Rows2 size={15} />,
              hidden: view.mode !== "data",
              onSelect: toggleDensity,
            },
            {
              label: rowNumbers ? "Hide row numbers" : "Show row numbers",
              icon: <ListOrdered size={15} />,
              hidden: view.mode !== "data",
              onSelect: toggleRowNumbers,
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
            focus={filterFocus}
            onApply={(e) => {
              e?.preventDefault();
              setView({
                filters: draftFilters.map(fromDraft).filter((f): f is RowFilter => f !== null),
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
              <button type="button" className="dbx-chip-edit" onClick={() => openFilters(i)} title="Edit this filter">
                <span className="font-medium">{f.column}</span> {opLabel(f.op)}
                {describeValue(f) ? <span className="mono"> {describeValue(f)}</span> : null}
              </button>
              <button
                aria-label="Remove filter"
                onClick={() => setView({ filters: view.filters.filter((_, j) => j !== i), page: 1 })}
              >
                <X size={11} />
              </button>
            </span>
          ))}
          <Button variant="ghost" size="sm" onClick={() => setView({ filters: [], page: 1 })}>
            Clear filters
          </Button>
        </div>
      ) : null}

      {view.mode === "structure" ? (
        <div className="dbx-structure-scroll">
          <SchemaView
            databaseId={databaseId}
            table={tab.name}
            dialect={dialect}
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
            {commandResult ? (
              <>
                <div className="dbx-cmd-result-bar" role="status">
                  <SquareTerminal size={14} aria-hidden />
                  <span className="font-medium text-sm">Query result</span>
                  <span className="text-sm muted">
                    {commandResult.result.row_count.toLocaleString()} row{commandResult.result.row_count === 1 ? "" : "s"} ·{" "}
                    {commandResult.result.duration_ms} ms
                    {commandResult.result.truncated ? " · stopped at the row limit; add a LIMIT" : ""}
                  </span>
                  <code className="dbx-cmd-result-sql text-sm muted hide-sm" title={commandResult.sql}>
                    {commandResult.sql.replace(/\s+/g, " ")}
                  </code>
                  <div className="flex-1" />
                  <Button size="sm" onClick={() => setCommandResult(null)}>
                    Back to table
                  </Button>
                </div>
                <ResultGrid key={commandResult.sql + commandResult.result.duration_ms} result={commandResult.result} dialect={dialect} />
              </>
            ) : isLoading ? (
              <GridSkeleton />
            ) : error ? (
              <div className="dbx-center">
                <EmptyState
                  title="Could not load rows"
                  hint={(error as Error).message}
                  action={
                    <Button onClick={() => void refetch()}>Try again</Button>
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
                incoming={incoming}
                onReferencing={referencing}
                editRequest={editRequest}
                onEditRequestDone={clearEditRequest}
                hidden={view.hidden}
                frozen={view.frozen}
                onHideColumn={(name) => setView({ hidden: [...(view.hidden ?? []), name] }, false)}
                onFreeze={(frozen) => setView({ frozen }, false)}
                onRowDetail={openRowPanel}
                onEditJson={editJsonInPanel}
                onFilterColumn={(name) => {
                  const c = columns.find((x) => x.name === name);
                  openFilters(undefined, filterFor(name, c ? cellKind(c.type) === "text" : false));
                }}
                density={density}
                rowNumbers={rowNumbers}
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
          {panel && !commandResult ? (
            <ValuePanel
              row={cursor ? rows[cursor.row] : undefined}
              col={cursor ? columns[cursor.col] : undefined}
              ci={cursor?.col ?? -1}
              editable={
                cursor && !columns[cursor.col]?.generated
                  ? rows[cursor.row]?.kind === "new" || (editable && rows[cursor.row]?.kind === "data")
                  : false
              }
              onApply={(v) => cursor && onEdit(cursor.row, cursor.col, v)}
              editRequest={panelEdit}
              onClose={togglePanel}
            />
          ) : null}
          {side === "row" && !commandResult ? (
            <RowPanel
              row={cursor ? rows[cursor.row] : undefined}
              label={(() => {
                const r = cursor ? rows[cursor.row] : undefined;
                return !r ? "Row" : r.kind === "new" ? "New row" : `Row ${offset + r.index + 1}`;
              })()}
              columns={columns}
              keyCols={key}
              editable={(() => {
                const r = cursor ? rows[cursor.row] : undefined;
                return r?.kind === "new" || (editable && r?.kind === "data" && !r.deleted);
              })()}
              onApply={(ci, v) => cursor && onEdit(cursor.row, ci, v)}
              onPrev={cursor && cursor.row > 0 ? () => setCursor({ row: cursor.row - 1, col: cursor.col }) : undefined}
              onNext={cursor && cursor.row + 1 < rows.length ? () => setCursor({ row: cursor.row + 1, col: cursor.col }) : undefined}
              onClose={toggleRowPanel}
            />
          ) : null}
          {historyOpen ? (
            <RowHistory databaseId={databaseId} table={tab.name} canWrite={canWrite} onClose={() => setHistoryOpen(false)} />
          ) : null}
          {compareOpen ? (
            <BackupCompare
              databaseId={databaseId}
              table={tab.name}
              canWrite={canWrite}
              canBackup={canBackup}
              onClose={() => setCompareOpen(false)}
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
            {counts.failed ? <span className="text-danger"> · {counts.failed} failed</span> : null}
          </span>
          {counts.failed ? (
            <span className="dbx-commit-error" title={Object.values(pending.errors).join("\n")}>
              {Object.values(pending.errors)[0]}
            </span>
          ) : null}
          <div className="flex-1" />
          <Button variant="ghost" size="sm" onClick={() => setReview(true)} disabled={saving}>
            Review SQL
          </Button>
          <Button size="sm" onClick={() => void discard()} disabled={saving}>
            Discard
          </Button>
          <Button size="sm" variant="primary" loading={saving} onClick={() => void doSave()}>
            Save <kbd className="dbx-kbd-primary hide-sm">Ctrl S</kbd>
          </Button>
        </div>
      ) : null}

      {view.mode === "data" && commandOpen ? (
        <CommandPanel
          databaseId={databaseId}
          table={tab.name}
          quotedTable={quoted}
          engine={engine}
          canWrite={canWrite}
          onResult={(sql, result) => setCommandResult({ sql, result })}
          onWrote={() => {
            setCommandResult(null);
            void refetch();
          }}
          onClose={toggleCommand}
        />
      ) : null}

      {view.mode === "data" ? (
        <div className="dbx-footer">
          <span className="text-sm muted">
            {commandResult ? (
              `Query result: ${commandResult.result.row_count.toLocaleString()} row${commandResult.result.row_count === 1 ? "" : "s"}`
            ) : (
              <>
                {data && data.rows.length > 0 ? <RowRange data={data} offset={offset} /> : data ? "No rows" : ""}
                {selected.size > 1 ? ` · ${selected.size} selected` : ""}
              </>
            )}
          </span>
          <div className="flex-1" />
          <label className="flex items-center gap-1 text-sm muted">
            <span className="hide-sm">Rows per page</span>
            <Select
              className="dbx-pagesize"
              value={view.pageSize}
              onChange={(e) => {
                setCommandResult(null);
                setView({ pageSize: Number(e.target.value), page: 1 });
              }}
              aria-label="Rows per page"
            >
              {PAGE_SIZES.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </Select>
          </label>
          <Pagination
            page={view.page}
            pageCount={pageCount}
            hasMore={hasFullPage}
            onPage={(page) => {
              setCommandResult(null);
              setView({ page });
            }}
          />
        </div>
      ) : null}

      {review ? (
        <Modal open wide onClose={() => setReview(false)} title={`Changes to ${tab.label}`}>
          <p className="text-sm muted mt-0">
            What saving will do, as SQL. Each row is applied on its own, in this order; if one fails, the others still
            go through and the failed one stays here for you to fix.
          </p>
          <pre className="dbx-review">
            {toMutations(pending)
              .map((m) => mutationSQL(m, quoted, columns, dialect))
              .join("\n")}
          </pre>
          <div className="flex gap-2 mt-3">
            <Button
              variant="primary"
              onClick={() => {
                setReview(false);
                void doSave();
              }}
            >
              Save {count} change{count === 1 ? "" : "s"}
            </Button>
            <Button onClick={() => setReview(false)}>Close</Button>
          </div>
        </Modal>
      ) : null}
      {importOpen ? <ImportCSV databaseId={databaseId} table={tab.name} onClose={() => setImportOpen(false)} /> : null}
    </div>
  );
}

/** isJson reports whether text parses as JSON. */
function isJson(text: string): boolean {
  try {
    JSON.parse(text);
    return true;
  } catch {
    return false;
  }
}

/** ValuePanel shows the selected cell in full and edits long or multi-line values. */
function ValuePanel({
  row,
  col,
  ci,
  editable,
  onApply,
  editRequest,
  onClose,
}: {
  row?: GridRow;
  col?: { name: string; type: string; nullable: boolean; has_default: boolean };
  ci: number;
  editable: boolean;
  onApply: (v: string | null) => void;
  /** A new id here starts editing the selected cell (a JSON cell edited from the grid). */
  editRequest: number | null;
  onClose: () => void;
}) {
  const st = row && col ? cellState(row, col, ci) : null;
  const [draft, setDraft] = useState<string | null>(null);
  const cellKey = row && col ? `${row.kind === "new" ? row.id : row.index}:${col.name}` : "";
  const [prevKey, setPrevKey] = useState(cellKey);
  const cellChanged = prevKey !== cellKey;
  if (cellChanged) {
    setPrevKey(cellKey);
    setDraft(null);
  }
  const { push } = useToast();
  const copy = useCopy();
  const kind = col ? cellKind(col.type) : "text";
  const locked = st ? isTruncated(st.original) && !st.changed : false;
  const jsonDoc = st ? parseJsonDoc(st.value) : undefined;
  // A JSON column, or a cell holding a JSON object or array, is edited as JSON.
  const jsonCell = kind === "json" || jsonDoc !== undefined;
  // Editing starts from the value, with a JSON object or array pretty-printed.
  const draftStart = st ? (jsonDoc !== undefined ? formatJson(st.value ?? "") : (st.value ?? "")) : "";
  const applyJson = (text: string) => {
    // Unchanged since it was formatted: keep the stored text as it is.
    // Otherwise valid JSON is saved on one line again, like it was stored.
    if (text !== draftStart) onApply(isJson(text) ? compactJson(text) : text);
    setDraft(null);
  };
  const cancelJson = () => setDraft(null);
  const [seenEditRequest, setSeenEditRequest] = useState<number | null>(null);
  if (editRequest !== seenEditRequest) {
    setSeenEditRequest(editRequest);
    // Keep a draft already being edited for this cell.
    if (editRequest !== null && st && editable && !locked && (draft === null || cellChanged)) setDraft(draftStart);
  }

  return (
    <aside className="dbx-valuepanel" aria-label="Value panel">
      <div className="dbx-valuepanel-head">
        <span className="truncate font-medium">{col ? col.name : "Value"}</span>
        {col ? <span className="muted text-xs truncate">{col.type}</span> : null}
        <div className="flex-1" />
        <IconButton label="Close value panel" onClick={onClose}>
          <X size={14} aria-hidden />
        </IconButton>
      </div>
      {!st ? (
        <p className="muted text-sm p-3">
          Select a cell to see its full value here.
        </p>
      ) : draft !== null ? (
        <div className="dbx-valuepanel-body">
          {jsonCell ? (
            <JsonEditor value={draft} onChange={setDraft} onApply={applyJson} onCancel={cancelJson} />
          ) : (
            <Textarea
              className="dbx-valuepanel-edit"
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
          )}
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="primary"
              onClick={() => {
                if (jsonCell) {
                  applyJson(draft);
                  return;
                }
                onApply(draft);
                setDraft(null);
              }}
            >
              Apply
            </Button>
            <Button size="sm" onClick={() => setDraft(null)}>
              Cancel
            </Button>
            {kind === "json" || parseJsonDoc(draft) ? (
              <Button variant="ghost" size="sm" onClick={() => setDraft(isJson(draft) ? formatJson(draft) : draft)}>
                Format JSON
              </Button>
            ) : null}
          </div>
          <p className="muted text-xs m-0">
            Ctrl+Enter applies. Nothing is written until you save the tab.
          </p>
        </div>
      ) : (
        <div className="dbx-valuepanel-body">
          {st.isDefault ? (
            <span className="dbx-pill">DEFAULT</span>
          ) : st.value === null ? (
            <span className="dbx-pill">NULL</span>
          ) : jsonDoc ? (
            <JsonViewer key={cellKey} doc={jsonDoc} />
          ) : (
            <pre className="dbx-valuepanel-pre">{prettyValue(st.value, kind)}</pre>
          )}
          <div className="flex flex-wrap gap-2">
            {st.value !== null ? (
              <Button
                size="sm"
                icon={<ClipboardCopy size={13} aria-hidden />}
                onClick={() => copy(st.value ?? "", "Copied value")}
              >
                Copy
              </Button>
            ) : null}
            {editable && !locked ? (
              <Button size="sm" onClick={() => setDraft(draftStart)}>
                Edit
              </Button>
            ) : null}
            {editable && col?.nullable && st.value !== null ? (
              <Button variant="ghost" size="sm" onClick={() => onApply(null)}>
                Set NULL
              </Button>
            ) : null}
          </div>
          {locked ? (
            <p className="muted text-xs m-0">
              Shown cut short at 4 KB, so it can&apos;t be edited here. Use the SQL console.
            </p>
          ) : st.changed ? (
            <p className="text-xs m-0 text-warning">
              Changed — was {st.original === null ? "NULL" : `“${st.original.slice(0, 80)}${st.original.length > 80 ? "…" : ""}”`}
            </p>
          ) : null}
        </div>
      )}
    </aside>
  );
}
