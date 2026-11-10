"use client";

import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";

import { useToast } from "@/components/toast";
import {
  cellKind,
  estimateWidth,
  isTruncated,
  parseJsonDoc,
  toInsertSQL,
  toJSON,
  toTSV,
  type CellKind,
  type Dialect,
} from "@/lib/data-browser/cells";
import { describeFilters, followFilters, referencingFilters, type RowRecord } from "@/lib/data-browser/fk";
import { frozenOffsets, VIRTUAL_FROM, virtualWindow, visibleColumns } from "@/lib/data-browser/tabs";
import type { BrowseColumn, ForeignKey, RowValues, SortKey } from "@/lib/types";
import {
  AlertCircle,
  ArrowDown,
  ArrowUp,
  ArrowUpRight,
  ClipboardCopy,
  CopyPlus,
  CornerDownRight,
  Eraser,
  EyeOff,
  Filter,
  KeyRound,
  Link2,
  Pin,
  RotateCcw,
  Trash2,
  Undo2,
} from "lucide-react";

import { useContextMenu, type ContextItem } from "./context-menu";
import { OverlayScrollbars } from "./overlay-scrollbars";

/** GridRow is one line of the grid: a pending new row or a fetched row. */
export type GridRow =
  | { kind: "new"; id: string; values: RowValues; error?: string }
  | {
      kind: "data";
      index: number;
      cells: (string | null)[];
      /** Null when the table has no usable key (read-only). */
      rowId: string | null;
      deleted: boolean;
      changes?: RowValues;
      error?: string;
    };

export type Cell = { row: number; col: number };

type Editing = { row: number; col: number; text: string; initial: string };

type CellState = { value: string | null; isDefault: boolean; changed: boolean; original: string | null };

export function cellState(row: GridRow, col: BrowseColumn, ci: number): CellState {
  if (row.kind === "new") {
    const has = col.name in row.values;
    return { value: has ? row.values[col.name] : null, isDefault: !has, changed: false, original: null };
  }
  const original = row.cells[ci];
  const has = row.changes !== undefined && col.name in row.changes;
  return { value: has ? row.changes![col.name] : original, isDefault: false, changed: has, original };
}

/** rowValues returns the row as currently shown (pending edits applied). */
export function rowValues(row: GridRow, columns: BrowseColumn[]): (string | null)[] {
  return columns.map((c, i) => cellState(row, c, i).value);
}

/** toRecord is rowValues by column name. */
export function toRecord(row: GridRow, columns: BrowseColumn[]): RowRecord {
  const out: RowRecord = {};
  columns.forEach((c, i) => (out[c.name] = cellState(row, c, i).value));
  return out;
}

const ROWNUM_WIDTH = 52;
const isTrue = (v: string) => /^(t|true|1|y|yes|on)$/i.test(v);

export type GridProps = {
  columns: BrowseColumn[];
  rows: GridRow[];
  keyCols: string[];
  offset: number;
  dialect: Dialect;
  quotedTable: string;
  sort: SortKey[];
  onSort: (column: string, dir: "asc" | "desc" | "toggle" | "clear", additive: boolean) => void;
  widths: Record<string, number>;
  onWidths: (w: Record<string, number>) => void;
  fks: Record<string, ForeignKey>;
  /** Existing rows can be edited (write access and a row key). */
  editable: boolean;
  cursor: Cell | null;
  setCursor: (c: Cell | null) => void;
  selected: Set<number>;
  setSelected: (s: Set<number>) => void;
  onEdit: (row: number, col: number, value: string | null | undefined) => void;
  onRevertCell: (row: number, col: number) => void;
  onDeleteRows: (rows: number[]) => void;
  onRestoreRows: (rows: number[]) => void;
  onDuplicate: (row: number) => void;
  /** Absent for query results, which cannot be filtered. */
  onFilterBy?: (column: string, value: string | null) => void;
  onFollow: (fk: ForeignKey, row: RowRecord) => void;
  /** Foreign keys of other tables that reference this one. */
  incoming: ForeignKey[];
  onReferencing: (fk: ForeignKey, row: RowRecord) => void;
  emptyText: string;
  /** Open this cell's editor as soon as it is on screen (a just-added row). */
  editRequest: Cell | null;
  onEditRequestDone: () => void;
  /** Columns hidden by name, and how many visible ones stay put when scrolling sideways. */
  hidden?: string[];
  frozen?: number;
  onHideColumn?: (column: string) => void;
  onFreeze?: (count: number) => void;
  /** Shift+Enter: show the cursor's row in full. */
  onRowDetail?: () => void;
  /** Editing a cell that holds JSON opens this (the value panel) instead of the inline editor. */
  onEditJson?: () => void;
  /** Header menu: start a filter on this column in the filter editor. */
  onFilterColumn?: (column: string) => void;
  density?: "compact" | "comfortable";
};

export function DataGrid(props: GridProps) {
  const { columns, rows, cursor, setCursor, selected, setSelected, editable, keyCols } = props;
  const scroller = useRef<HTMLDivElement>(null);
  const [editing, setEditingState] = useState<Editing | null>(null);
  // The ref is the source of truth while committing: moving focus back to the
  // grid blurs the input, and that blur must not commit a second time.
  const editingRef = useRef<Editing | null>(null);
  const setEditing = useCallback((v: Editing | null) => {
    editingRef.current = v;
    setEditingState(v);
  }, []);
  const onEditText = useCallback(
    (text: string) => {
      if (editingRef.current) setEditing({ ...editingRef.current, text });
    },
    [setEditing],
  );
  const [live, setLive] = useState<Record<string, number>>({});
  const anchor = useRef<number | null>(null);
  const ctx = useContextMenu();
  const { push } = useToast();
  const kinds = useMemo(() => columns.map((c) => cellKind(c.type)), [columns]);
  // Indexes of the columns drawn, in order; cells keep their index into
  // columns so row values and edits are unaffected by hiding.
  const hiddenKey = (props.hidden ?? []).join("\u0000");
  const vis = useMemo(
    () => visibleColumns(columns.map((c) => c.name), props.hidden),
    // hiddenKey stands for props.hidden.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [columns, hiddenKey],
  );

  // Starting widths are estimated once per column set, so paging does not
  // make columns jump.
  const colKey = columns.map((c) => c.name).join("\u0000");
  const [estimates, setEstimates] = useState<{ key: string; widths: Record<string, number> }>({ key: "", widths: {} });
  let estimated = estimates.widths;
  if (estimates.key !== colKey) {
    estimated = {};
    for (const [i, c] of columns.entries()) {
      estimated[c.name] = estimateWidth(
        c,
        rows.map((r) => cellState(r, c, i).value),
      );
    }
    setEstimates({ key: colKey, widths: estimated });
  }
  const widthOf = (name: string) => live[name] ?? props.widths[name] ?? estimated[name] ?? 140;
  const total = ROWNUM_WIDTH + vis.reduce((s, i) => s + widthOf(columns[i].name), 0);
  const frozen = Math.min(props.frozen ?? 0, Math.max(0, vis.length - 1));
  const lefts = frozenOffsets(
    vis.slice(0, frozen).map((i) => widthOf(columns[i].name)),
    frozen,
    ROWNUM_WIDTH,
  );
  const leftsKey = lefts.join(",");
  // Stable for the memoized rows unless the offsets really change.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const stableLefts = useMemo(() => lefts, [leftsKey]);

  const canEditRow = useCallback(
    (r: number) => {
      const row = rows[r];
      if (!row) return false;
      return row.kind === "new" || (editable && !row.deleted);
    },
    [rows, editable],
  );

  // ---- virtualization: long pages draw only the rows in view ----
  const virtual = rows.length > VIRTUAL_FROM;
  const [viewport, setViewport] = useState({ top: 0, height: 800 });
  const [metrics, setMetrics] = useState({ row: 30, head: 44 });
  const frame = useRef(0);
  const onScroll = useCallback(() => {
    if (!virtual) return;
    cancelAnimationFrame(frame.current);
    frame.current = requestAnimationFrame(() => {
      const el = scroller.current;
      if (el) setViewport({ top: el.scrollTop, height: el.clientHeight });
    });
  }, [virtual]);
  useEffect(() => {
    const el = scroller.current;
    if (!el || !virtual) return;
    const ro = new ResizeObserver(() => setViewport({ top: el.scrollTop, height: el.clientHeight }));
    ro.observe(el);
    return () => ro.disconnect();
  }, [virtual]);
  // Row and header heights follow the density and the font; measure them.
  useLayoutEffect(() => {
    const el = scroller.current;
    const tr = el?.querySelector<HTMLElement>("tbody tr.dbx-row");
    const head = el?.querySelector<HTMLElement>("thead");
    if (!tr || !head) return;
    const row = tr.getBoundingClientRect().height;
    const h = head.getBoundingClientRect().height;
    if (row > 0 && (Math.abs(row - metrics.row) > 0.5 || Math.abs(h - metrics.head) > 0.5)) setMetrics({ row, head: h });
  }, [rows, props.density, metrics.row, metrics.head]);
  const win = virtual
    ? virtualWindow(rows.length, metrics.row, viewport.top - metrics.head, viewport.height)
    : { start: 0, end: rows.length };

  // Keep the cursor cell in view while moving with the keyboard.
  useEffect(() => {
    if (!cursor) return;
    const el = scroller.current;
    const cell = el?.querySelector(`[data-cell="${cursor.row}:${cursor.col}"]`);
    if (cell) {
      cell.scrollIntoView({ block: "nearest", inline: "nearest" });
    } else if (el && virtual) {
      // Not drawn yet: scroll its row into view; the next render draws it.
      const top = cursor.row * metrics.row;
      if (top < el.scrollTop) el.scrollTop = top;
      else if (top + metrics.row > el.scrollTop + el.clientHeight - metrics.head)
        el.scrollTop = top + metrics.row - el.clientHeight + metrics.head;
    }
  }, [cursor, virtual, metrics]);

  function startEdit(r: number, c: number, text?: string) {
    if (!canEditRow(r)) return;
    if (columns[c]?.generated) {
      push("info", `${columns[c].name} is computed by the database and cannot be edited.`);
      return;
    }
    const st = cellState(rows[r], columns[c], c);
    if (isTruncated(st.original) && !st.changed) {
      push("info", "This value is too long to edit here. Use the SQL console to change it.");
      return;
    }
    // JSON is edited in the value panel, which has room for it and Format JSON.
    if (props.onEditJson && (cellKind(columns[c].type) === "json" || parseJsonDoc(st.value))) {
      setCursor({ row: r, col: c });
      props.onEditJson();
      return;
    }
    const initial = st.value ?? "";
    setEditing({ row: r, col: c, initial: st.value === null ? "\u0000null" : initial, text: text ?? initial });
  }

  function commitEdit(move?: "down" | "right" | "left") {
    const ed = editingRef.current;
    if (!ed) return;
    const { row, col, text, initial } = ed;
    setEditing(null);
    const wasNull = initial === "\u0000null";
    if (!(wasNull && text === "") && text !== initial) props.onEdit(row, col, text);
    const pos = vis.indexOf(col);
    if (move === "down" && row + 1 < rows.length) setCursor({ row: row + 1, col });
    else if (move === "right" && pos + 1 < vis.length) setCursor({ row, col: vis[pos + 1] });
    else if (move === "left" && pos > 0) setCursor({ row, col: vis[pos - 1] });
    scroller.current?.focus({ preventScroll: true });
  }

  function cancelEdit() {
    setEditing(null);
    scroller.current?.focus({ preventScroll: true });
  }

  function copy(text: string, what: string) {
    void navigator.clipboard
      ?.writeText(text)
      .then(() => push("success", `Copied ${what}`))
      .catch(() => push("error", "The browser did not allow copying"));
  }

  const selectedRows = () => (selected.size ? [...selected].sort((a, b) => a - b) : cursor ? [cursor.row] : []);
  const valuesOf = (idx: number[]) => idx.map((i) => rowValues(rows[i], columns));

  function copySelection() {
    if (selected.size > 0) {
      const idx = selectedRows();
      copy(toTSV(columns, valuesOf(idx)), `${idx.length} row${idx.length === 1 ? "" : "s"}`);
    } else if (cursor) {
      copy(cellState(rows[cursor.row], columns[cursor.col], cursor.col).value ?? "", "value");
    }
  }

  function onKeyDown(e: React.KeyboardEvent) {
    if (editing) return;
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.key.toLowerCase() === "c") {
      e.preventDefault();
      copySelection();
      return;
    }
    if (mod && e.key.toLowerCase() === "a") {
      e.preventDefault();
      setSelected(new Set(rows.map((_, i) => i)));
      return;
    }
    if (rows.length === 0 || vis.length === 0) return;
    const cur = cursor ?? { row: 0, col: vis[0] };
    const pos = Math.max(0, vis.indexOf(cur.col));
    const at = (p: number) => vis[Math.max(0, Math.min(vis.length - 1, p))];
    const move = (row: number, col: number) => {
      e.preventDefault();
      const next = { row: Math.max(0, Math.min(rows.length - 1, row)), col: Math.max(0, Math.min(columns.length - 1, col)) };
      if (e.shiftKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
        const a = anchor.current ?? cur.row;
        anchor.current = a;
        const s = new Set<number>();
        for (let i = Math.min(a, next.row); i <= Math.max(a, next.row); i++) s.add(i);
        setSelected(s);
      } else if (!e.shiftKey) {
        anchor.current = next.row;
        if (selected.size) setSelected(new Set());
      }
      setCursor(next);
    };
    switch (e.key) {
      case "ArrowDown":
        return move(cursor ? cur.row + 1 : 0, cur.col);
      case "ArrowUp":
        return move(cur.row - 1, cur.col);
      case "ArrowRight":
        return move(cur.row, cursor ? at(pos + 1) : vis[0]);
      case "ArrowLeft":
        return move(cur.row, at(pos - 1));
      case "Home":
        return move(mod ? 0 : cur.row, vis[0]);
      case "End":
        return move(mod ? rows.length - 1 : cur.row, vis[vis.length - 1]);
      case "PageDown":
        if (e.altKey) return; // Alt+PageDown turns the page (table tab)
        return move(cur.row + 15, cur.col);
      case "PageUp":
        if (e.altKey) return;
        return move(cur.row - 15, cur.col);
      case "Tab":
        if (!cursor) return;
        return move(cur.row, at(pos + (e.shiftKey ? -1 : 1)));
      case "Enter":
        if (e.shiftKey && cursor && props.onRowDetail) {
          e.preventDefault();
          props.onRowDetail();
          return;
        }
      // falls through
      case "F2":
        if (cursor) {
          e.preventDefault();
          startEdit(cursor.row, cursor.col);
        }
        return;
      case "Escape":
        if (selected.size) {
          e.stopPropagation();
          setSelected(new Set());
        }
        return;
      case "Delete":
      case "Backspace": {
        // Only rows picked on purpose (row numbers, Shift+arrows): a stray
        // Backspace on a cell must not mark its row for deletion.
        const idx = [...selected].filter((i) => rows[i]?.kind === "new" || (editable && rows[i]?.kind === "data"));
        if (idx.length) {
          e.preventDefault();
          props.onDeleteRows(idx);
        }
        return;
      }
    }
    // Typing a character starts editing the cell with it, like a spreadsheet.
    if (cursor && !mod && !e.altKey && e.key.length === 1) {
      e.preventDefault();
      e.stopPropagation(); // keep "/", "?" and "g" from the global shortcuts
      startEdit(cursor.row, cursor.col, e.key);
    }
  }

  function onRowNumber(e: React.MouseEvent, r: number) {
    let next: Set<number>;
    if (e.shiftKey && anchor.current !== null) {
      next = new Set();
      for (let i = Math.min(anchor.current, r); i <= Math.max(anchor.current, r); i++) next.add(i);
    } else if (e.ctrlKey || e.metaKey) {
      next = new Set(selected);
      if (next.has(r)) next.delete(r);
      else next.add(r);
      anchor.current = r;
    } else {
      next = new Set([r]);
      anchor.current = r;
    }
    setSelected(next);
    setCursor({ row: r, col: cursor?.col ?? 0 });
    scroller.current?.focus({ preventScroll: true });
  }

  function cellMenu(e: React.MouseEvent, r: number, c: number) {
    const row = rows[r];
    const col = columns[c];
    const st = cellState(row, col, c);
    // Right-clicking outside the selection acts on that row alone.
    const inSel = selected.has(r);
    if (!inSel && selected.size) setSelected(new Set());
    setCursor({ row: r, col: c });
    const idx = inSel ? selectedRows() : [r];
    const n = idx.length;
    const rowsLabel = n === 1 ? "row" : `${n} rows`;
    const fk = props.fks[col.name];
    const record = toRecord(row, columns);
    const editableHere = canEditRow(r) && !col.generated;
    const deletable = idx.filter((i) => rows[i].kind === "new" || (editable && rows[i].kind === "data" && !(rows[i] as { deleted: boolean }).deleted));
    const restorable = idx.filter((i) => rows[i].kind === "data" && (rows[i] as { deleted: boolean }).deleted);
    const items: ContextItem[] = [
      { label: "Copy value", icon: <ClipboardCopy size={14} />, hint: "Ctrl C", onSelect: () => copy(st.value ?? "", "value") },
      { label: `Copy ${rowsLabel} as TSV`, onSelect: () => copy(toTSV(columns, valuesOf(idx), true), rowsLabel) },
      { label: `Copy ${rowsLabel} as JSON`, onSelect: () => copy(toJSON(columns, valuesOf(idx)), rowsLabel) },
      {
        label: `Copy ${rowsLabel} as SQL INSERT`,
        onSelect: () => copy(toInsertSQL(props.quotedTable, columns, valuesOf(idx), props.dialect), rowsLabel),
      },
      "separator",
      {
        label: st.value === null ? `Filter: ${col.name} is NULL` : `Filter: ${col.name} = ${short(st.value)}`,
        icon: <Filter size={14} />,
        hidden: st.isDefault || row.kind === "new" || !props.onFilterBy,
        onSelect: () => props.onFilterBy?.(col.name, st.value),
      },
      {
        label: `Open referenced row in ${fk?.ref_table}`,
        icon: <ArrowUpRight size={14} />,
        hidden: !fk || row.kind !== "data" || !followFilters(fk, record),
        onSelect: () => fk && props.onFollow(fk, record),
      },
      ...props.incoming.map((ref): ContextItem => ({
        label: `Rows in ${ref.table} referencing this row (${ref.columns.join(", ")})`,
        icon: <CornerDownRight size={14} />,
        hidden: row.kind !== "data" || !referencingFilters(ref, record),
        onSelect: () => props.onReferencing(ref, record),
      })),
      "separator",
      { label: "Edit value", hint: "Enter", hidden: !editableHere, onSelect: () => startEdit(r, c) },
      { label: "Set to NULL", icon: <Eraser size={14} />, hidden: !editableHere || !col.nullable || (st.value === null && !st.isDefault), onSelect: () => props.onEdit(r, c, null) },
      { label: "Set to DEFAULT", hidden: row.kind !== "new" || !col.has_default || st.isDefault, onSelect: () => props.onEdit(r, c, undefined) },
      { label: "Revert value", icon: <Undo2 size={14} />, hidden: !st.changed, onSelect: () => props.onRevertCell(r, c) },
      { label: "Duplicate row", icon: <CopyPlus size={14} />, hidden: !editable || n !== 1, onSelect: () => props.onDuplicate(r) },
      {
        label: deletable.length === 1 ? "Delete row" : `Delete ${deletable.length} rows`,
        icon: <Trash2 size={14} />,
        danger: true,
        hint: "Del",
        hidden: deletable.length === 0,
        onSelect: () => props.onDeleteRows(deletable),
      },
      {
        label: restorable.length === 1 ? "Restore row" : `Restore ${restorable.length} rows`,
        icon: <RotateCcw size={14} />,
        hidden: restorable.length === 0,
        onSelect: () => props.onRestoreRows(restorable),
      },
    ];
    ctx.open(e, items);
  }

  function headerMenu(e: React.MouseEvent, col: BrowseColumn) {
    ctx.open(e, [
      { label: "Sort ascending", icon: <ArrowUp size={14} />, onSelect: () => props.onSort(col.name, "asc", false) },
      { label: "Sort descending", icon: <ArrowDown size={14} />, onSelect: () => props.onSort(col.name, "desc", false) },
      {
        label: "Clear sort",
        hidden: !props.sort.some((s) => s.column === col.name),
        onSelect: () => props.onSort(col.name, "clear", true),
      },
      "separator",
      {
        label: "Filter…",
        icon: <Filter size={14} />,
        hidden: !props.onFilterColumn,
        onSelect: () => props.onFilterColumn?.(col.name),
      },
      { label: "Filter: is NULL", hidden: !col.nullable || !props.onFilterBy, onSelect: () => props.onFilterBy?.(col.name, null) },
      { label: "Fit width to content", onSelect: () => autoFit(col) },
      "separator",
      {
        label: "Hide column",
        icon: <EyeOff size={14} />,
        hidden: !props.onHideColumn || vis.length <= 1,
        onSelect: () => props.onHideColumn?.(col.name),
      },
      {
        label: frozen > 0 && vis.indexOf(columns.indexOf(col)) === frozen - 1 ? "Unfreeze columns" : "Freeze columns up to here",
        icon: <Pin size={14} />,
        hidden: !props.onFreeze || vis.indexOf(columns.indexOf(col)) >= vis.length - 1,
        onSelect: () => {
          const p = vis.indexOf(columns.indexOf(col)) + 1;
          props.onFreeze?.(frozen === p ? 0 : p);
        },
      },
      {
        label: "Unfreeze columns",
        hidden: !props.onFreeze || frozen === 0 || vis.indexOf(columns.indexOf(col)) === frozen - 1,
        onSelect: () => props.onFreeze?.(0),
      },
      { label: "Copy column name", icon: <ClipboardCopy size={14} />, onSelect: () => copy(col.name, col.name) },
    ]);
  }

  function autoFit(col: BrowseColumn) {
    const i = columns.indexOf(col);
    const w = Math.min(640, estimateWidth(col, rows.map((r) => cellState(r, col, i).value)) * 1.15);
    props.onWidths({ ...props.widths, [col.name]: Math.round(w) });
  }

  function startResize(e: React.PointerEvent, col: BrowseColumn) {
    e.preventDefault();
    e.stopPropagation();
    const startX = e.clientX;
    const start = widthOf(col.name);
    const target = e.currentTarget as HTMLElement;
    target.setPointerCapture(e.pointerId);
    let w = start;
    const onMove = (ev: PointerEvent) => {
      w = Math.max(50, Math.min(1600, start + ev.clientX - startX));
      setLive({ [col.name]: w });
    };
    const onUp = () => {
      target.removeEventListener("pointermove", onMove);
      target.removeEventListener("pointerup", onUp);
      target.removeEventListener("pointercancel", onUp);
      setLive({});
      if (w !== start) props.onWidths({ ...props.widths, [col.name]: Math.round(w) });
    };
    target.addEventListener("pointermove", onMove);
    target.addEventListener("pointerup", onUp);
    target.addEventListener("pointercancel", onUp);
  }

  // Stable callbacks for the memoized rows.
  const handlers = useRef({ cellMenu, startEdit, onRowNumber, setCursor, follow: props.onFollow });
  useLayoutEffect(() => {
    handlers.current = { cellMenu, startEdit, onRowNumber, setCursor, follow: props.onFollow };
  });
  const onCellDown = useCallback((r: number, c: number) => handlers.current.setCursor({ row: r, col: c }), []);
  const onCellDouble = useCallback((r: number, c: number) => handlers.current.startEdit(r, c), []);
  const onCellMenu = useCallback((e: React.MouseEvent, r: number, c: number) => handlers.current.cellMenu(e, r, c), []);
  const onNum = useCallback((e: React.MouseEvent, r: number) => handlers.current.onRowNumber(e, r), []);
  const onFollow = useCallback((fk: ForeignKey, row: RowRecord) => handlers.current.follow(fk, row), []);

  const { editRequest, onEditRequestDone } = props;
  useEffect(() => {
    if (!editRequest || !rows[editRequest.row]) return;
    onEditRequestDone();
    setCursor(editRequest);
    handlers.current.startEdit(editRequest.row, editRequest.col);
  }, [editRequest, rows, onEditRequestDone, setCursor]);

  const sortIndex = (name: string) => props.sort.findIndex((s) => s.column === name);

  return (
    <div className="dbx-grid-frame">
      <div
        className={`dbx-grid${props.density === "comfortable" ? " is-comfortable" : ""}`}
        ref={scroller}
        tabIndex={0}
        role="grid"
        aria-rowcount={rows.length}
        aria-colcount={vis.length}
        onKeyDown={onKeyDown}
        onScroll={onScroll}
      >
        <table className="dbx-table" style={{ width: total }}>
          <colgroup>
            <col style={{ width: ROWNUM_WIDTH }} />
            {vis.map((i) => (
              <col key={columns[i].name} style={{ width: widthOf(columns[i].name) }} />
            ))}
          </colgroup>
          <thead>
            <tr>
              <th className="dbx-rownum dbx-corner" scope="col">
                <span className="sr-only">Row</span>
              </th>
              {vis.map((ci, pos) => {
                const c = columns[ci];
                const si = sortIndex(c.name);
                const s = si >= 0 ? props.sort[si] : null;
                const isKey = keyCols.includes(c.name);
                const fk = props.fks[c.name];
                return (
                  <th
                    key={c.name}
                    scope="col"
                    className={`dbx-th${kinds[ci] === "number" ? " num" : ""}${pos < frozen ? " is-frozen" : ""}${pos === frozen - 1 ? " is-frozen-edge" : ""}`}
                    style={pos < frozen ? { left: lefts[pos] } : undefined}
                    aria-sort={s ? (s.desc ? "descending" : "ascending") : undefined}
                    onContextMenu={(e) => headerMenu(e, c)}
                  >
                    <button
                      type="button"
                      className="dbx-th-button"
                      onClick={(e) => props.onSort(c.name, "toggle", e.shiftKey)}
                      title={`${c.name} · ${c.type}${c.nullable ? "" : " · NOT NULL"}${isKey ? " · key" : ""}${fk ? ` · → ${fk.ref_table}` : ""}\nClick to sort, Shift+click to add a sort column`}
                    >
                      <span className="dbx-th-name">
                        {isKey ? <KeyRound size={11} className="dbx-key" /> : null}
                        {fk ? <Link2 size={11} className="dbx-fkmark" /> : null}
                        <span className="truncate">{c.name}</span>
                        {s ? (
                          <span className="dbx-sort">
                            {s.desc ? <ArrowDown size={11} /> : <ArrowUp size={11} />}
                            {props.sort.length > 1 ? si + 1 : null}
                          </span>
                        ) : null}
                      </span>
                      <span className="dbx-th-type truncate">{c.type}</span>
                    </button>
                    <span
                      className="dbx-col-resize"
                      onPointerDown={(e) => startResize(e, c)}
                      onDoubleClick={(e) => {
                        e.stopPropagation();
                        autoFit(c);
                      }}
                      aria-hidden
                    />
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {win.start > 0 ? (
              <tr className="dbx-spacer" aria-hidden>
                <td colSpan={vis.length + 1} style={{ height: win.start * metrics.row }} />
              </tr>
            ) : null}
            {rows.slice(win.start, win.end).map((row, k) => {
              const r = win.start + k;
              return (
                <GridRowView
                  key={row.kind === "new" ? row.id : `d${row.index}`}
                  row={row}
                  r={r}
                  num={row.kind === "new" ? null : props.offset + row.index + 1}
                  columns={columns}
                  vis={vis}
                  lefts={stableLefts}
                  kinds={kinds}
                  fks={props.fks}
                  cursorCol={cursor?.row === r ? cursor.col : -1}
                  selected={selected.has(r)}
                  editing={editing?.row === r ? editing : null}
                  onEditText={onEditText}
                  onCommit={commitEdit}
                  onCancel={cancelEdit}
                  onCellDown={onCellDown}
                  onCellDouble={onCellDouble}
                  onCellMenu={onCellMenu}
                  onRowNumber={onNum}
                  onFollow={onFollow}
                />
              );
            })}
            {win.end < rows.length ? (
              <tr className="dbx-spacer" aria-hidden>
                <td colSpan={vis.length + 1} style={{ height: (rows.length - win.end) * metrics.row }} />
              </tr>
            ) : null}
          </tbody>
        </table>
        {rows.length === 0 ? <div className="dbx-grid-empty">{props.emptyText}</div> : null}
        {ctx.element}
      </div>
      <OverlayScrollbars target={scroller} />
    </div>
  );
}

const short = (v: string) => (v.length > 24 ? `${v.slice(0, 24)}…` : v);

type RowViewProps = {
  row: GridRow;
  r: number;
  num: number | null;
  columns: BrowseColumn[];
  /** Column indexes to draw, in order. */
  vis: number[];
  /** Sticky left offsets of the frozen (first) visible columns. */
  lefts: number[];
  kinds: CellKind[];
  fks: Record<string, ForeignKey>;
  cursorCol: number;
  selected: boolean;
  editing: { col: number; text: string } | null;
  onEditText: (text: string) => void;
  onCommit: (move?: "down" | "right" | "left") => void;
  onCancel: () => void;
  onCellDown: (r: number, c: number) => void;
  onCellDouble: (r: number, c: number) => void;
  onCellMenu: (e: React.MouseEvent, r: number, c: number) => void;
  onRowNumber: (e: React.MouseEvent, r: number) => void;
  onFollow: (fk: ForeignKey, row: RowRecord) => void;
};

const GridRowView = memo(
  function GridRowView(p: RowViewProps) {
    const { row, r } = p;
    const deleted = row.kind === "data" && row.deleted;
    const cls = [
      "dbx-row",
      row.kind === "new" ? "is-new" : "",
      deleted ? "is-deleted" : "",
      p.selected ? "is-selected" : "",
      row.error ? "has-error" : "",
    ]
      .filter(Boolean)
      .join(" ");
    // Where each outgoing key of this row leads (null: a key column is NULL).
    const record = row.kind === "data" ? toRecord(row, p.columns) : null;
    const follow: Record<string, ReturnType<typeof followFilters>> = {};
    if (record) for (const fk of Object.values(p.fks)) follow[fk.name] ??= followFilters(fk, record);
    return (
      <tr className={cls} aria-selected={p.selected} title={row.error}>
        <th scope="row" className="dbx-rownum" onMouseDown={(e) => e.preventDefault()} onClick={(e) => p.onRowNumber(e, r)}>
          {row.error ? <AlertCircle size={12} className="dbx-row-error" /> : null}
          {p.num === null ? <span className="dbx-new-mark">new</span> : p.num}
        </th>
        {p.vis.map((ci, pos) => {
          const c = p.columns[ci];
          const st = cellState(row, c, ci);
          const kind = p.kinds[ci];
          const isCursor = p.cursorCol === ci;
          const fk = row.kind === "data" ? p.fks[c.name] : undefined;
          const ed = p.editing && p.editing.col === ci ? p.editing : null;
          const cls = [
            "dbx-cell",
            kind === "number" ? "num" : "",
            kind === "json" || kind === "temporal" ? "mono" : "",
            st.changed ? "is-changed" : "",
            isCursor ? "is-cursor" : "",
            ed ? "is-editing" : "",
            pos < p.lefts.length ? "is-frozen" : "",
            pos === p.lefts.length - 1 ? "is-frozen-edge" : "",
          ]
            .filter(Boolean)
            .join(" ");
          return (
            <td
              key={c.name}
              className={cls}
              style={pos < p.lefts.length ? { left: p.lefts[pos] } : undefined}
              data-cell={`${r}:${ci}`}
              onMouseDown={() => p.onCellDown(r, ci)}
              onDoubleClick={() => p.onCellDouble(r, ci)}
              onContextMenu={(e) => p.onCellMenu(e, r, ci)}
              title={!ed && st.value && st.value.length > 40 ? st.value.slice(0, 600) : undefined}
            >
              {ed ? (
                <input
                  className="dbx-cell-input"
                  autoFocus
                  value={ed.text}
                  onFocus={(e) => {
                    // Typing-to-edit starts with one character: put the caret after it.
                    const el = e.currentTarget;
                    el.setSelectionRange(el.value.length, el.value.length);
                  }}
                  onChange={(e) => p.onEditText(e.target.value)}
                  onBlur={() => p.onCommit()}
                  onKeyDown={(e) => {
                    e.stopPropagation();
                    if (e.key === "Enter") {
                      e.preventDefault();
                      p.onCommit("down");
                    } else if (e.key === "Tab") {
                      e.preventDefault();
                      p.onCommit(e.shiftKey ? "left" : "right");
                    } else if (e.key === "Escape") {
                      e.preventDefault();
                      p.onCancel();
                    }
                  }}
                  aria-label={`Edit ${c.name}`}
                />
              ) : (
                <>
                  <CellContent value={st.value} isDefault={st.isDefault} kind={kind} />
                  {fk && follow[fk.name] ? (
                    <button
                      type="button"
                      className="dbx-fk"
                      tabIndex={-1}
                      title={`Open ${fk.ref_table} where ${describeFilters(follow[fk.name]!)}`}
                      onMouseDown={(e) => e.stopPropagation()}
                      onClick={(e) => {
                        e.stopPropagation();
                        p.onFollow(fk, record!);
                      }}
                    >
                      <ArrowUpRight size={12} />
                    </button>
                  ) : null}
                </>
              )}
            </td>
          );
        })}
      </tr>
    );
  },
  (a, b) =>
    a.row === b.row &&
    a.r === b.r &&
    a.num === b.num &&
    a.columns === b.columns &&
    a.vis === b.vis &&
    a.lefts === b.lefts &&
    a.kinds === b.kinds &&
    a.fks === b.fks &&
    a.cursorCol === b.cursorCol &&
    a.selected === b.selected &&
    a.editing === b.editing &&
    // Editing callbacks only matter while this row is being edited.
    (a.editing === null || (a.onCommit === b.onCommit && a.onEditText === b.onEditText)),
);

function CellContent({ value, isDefault, kind }: { value: string | null; isDefault: boolean; kind: CellKind }) {
  if (isDefault) return <span className="dbx-pill">DEFAULT</span>;
  if (value === null) return <span className="dbx-pill">NULL</span>;
  if (kind === "bool") {
    const on = isTrue(value);
    return <span className={`dbx-bool${on ? " on" : ""}`}>{on ? "true" : "false"}</span>;
  }
  if (value === "") return <span className="dbx-empty">empty</span>;
  const text = value.length > 300 ? `${value.slice(0, 300)}…` : value;
  return <span className="dbx-text">{text.replace(/\r?\n/g, " ↵ ")}</span>;
}
