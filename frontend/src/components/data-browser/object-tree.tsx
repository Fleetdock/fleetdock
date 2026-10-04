"use client";

import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState, type ReactNode } from "react";

import { useToast } from "@/components/toast";
import { Spinner } from "@/components/ui";
import { compactCount, quoteTable, type Dialect } from "@/lib/data-browser/cells";
import type { OpenRequest } from "@/lib/data-browser/tabs";
import { formatBytes } from "@/lib/format";
import { exportTableCSV } from "@/lib/hooks";
import type { DBObject, TableInfo } from "@/lib/types";
import {
  ChevronRight,
  Copy,
  Download,
  Eye,
  Folder,
  FolderOpen,
  Layers,
  Plus,
  RefreshCw,
  Search,
  SquareTerminal,
  Table2,
  TableProperties,
  X,
} from "lucide-react";

import { useContextMenu } from "./context-menu";

export type TreeItem = {
  kind: "table" | "view";
  /** view kind detail: "materialized_view" for those. */
  sub?: string;
  schema: string;
  /** Bare name. */
  label: string;
  /** Identifier the API takes. */
  name: string;
  rows?: number;
  bytes?: number;
  comment?: string;
};

/** buildItems merges base tables and views into one sorted list. */
export function buildItems(tables: TableInfo[], objects: DBObject[], dialect: Dialect): TreeItem[] {
  const api = (schema: string, name: string) => (dialect === "postgres" ? `${schema}.${name}` : name);
  const items: TreeItem[] = tables.map((t) => ({
    kind: "table",
    schema: t.schema,
    label: t.name,
    name: api(t.schema, t.name),
    rows: t.row_count,
    bytes: t.data_bytes + t.index_bytes,
    comment: t.comment || undefined,
  }));
  for (const o of objects) {
    if (o.kind !== "view" && o.kind !== "materialized_view") continue;
    items.push({ kind: "view", sub: o.kind, schema: o.schema, label: o.name, name: api(o.schema, o.name) });
  }
  return items.sort(
    (a, b) => a.schema.localeCompare(b.schema) || (a.kind === b.kind ? 0 : a.kind === "table" ? -1 : 1) || a.label.localeCompare(b.label),
  );
}

/** Highlight marks the matched part of a name. */
function Highlight({ text, query }: { text: string; query: string }) {
  if (!query) return <>{text}</>;
  const i = text.toLowerCase().indexOf(query);
  if (i < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, i)}
      <mark className="dbx-mark">{text.slice(i, i + query.length)}</mark>
      {text.slice(i + query.length)}
    </>
  );
}

export type ObjectTreeHandle = { focusSearch: () => void };

export const ObjectTree = forwardRef<
  ObjectTreeHandle,
  {
    databaseId: string;
    items: TreeItem[];
    isLoading: boolean;
    error?: string;
    dialect: Dialect;
    activeName: string | null;
    canWrite: boolean;
    onOpen: (req: OpenRequest) => void;
    onNewQuery: (sql?: string) => void;
    onCreateTable: () => void;
    onRefresh: () => void;
    refreshing: boolean;
  }
>(function ObjectTree(
  {
    databaseId,
    items,
    isLoading,
    error,
    dialect,
    activeName,
    canWrite,
    onOpen,
    onNewQuery,
    onCreateTable,
    onRefresh,
    refreshing,
  },
  ref,
) {
  const [query, setQuery] = useState("");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [cursor, setCursor] = useState(-1);
  const searchRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const ctx = useContextMenu();
  const { push } = useToast();

  useImperativeHandle(ref, () => ({ focusSearch: () => searchRef.current?.select() }), []);

  const q = query.trim().toLowerCase();
  const schemas = useMemo(() => [...new Set(items.map((i) => i.schema))], [items]);
  const grouped = schemas.length > 1;
  const matches = useMemo(
    () => (q ? items.filter((i) => i.label.toLowerCase().includes(q) || `${i.schema}.${i.label}`.toLowerCase().includes(q)) : items),
    [items, q],
  );
  // What is on screen, in order: collapsed schemas hide their items unless searching.
  const visible = useMemo(
    () => (grouped && !q ? matches.filter((i) => !collapsed.has(i.schema)) : matches),
    [matches, grouped, q, collapsed],
  );

  useEffect(() => {
    if (cursor < 0) return;
    listRef.current?.querySelector(`[data-index="${cursor}"]`)?.scrollIntoView({ block: "nearest" });
  }, [cursor]);

  const open = (item: TreeItem, preview: boolean) =>
    onOpen({ kind: item.kind, name: item.name, label: item.label, preview });

  function onSearchKey(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setCursor((c) => Math.min(c + 1, visible.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setCursor((c) => Math.max(c - 1, 0));
    } else if (e.key === "Enter") {
      const item = visible[cursor >= 0 ? cursor : 0];
      if (item) {
        e.preventDefault();
        open(item, false);
      }
    } else if (e.key === "Escape" && query) {
      e.stopPropagation();
      setQuery("");
      setCursor(-1);
    }
  }

  function menuFor(item: TreeItem) {
    const quoted = quoteTable(item.name, dialect, dialect === "postgres" ? item.schema : undefined);
    return [
      { label: "Open", icon: <Table2 size={14} />, onSelect: () => open(item, false) },
      {
        label: "Open structure",
        icon: <TableProperties size={14} />,
        onSelect: () => onOpen({ kind: item.kind, name: item.name, label: item.label, view: { mode: "structure" } }),
      },
      {
        label: "New query",
        icon: <SquareTerminal size={14} />,
        onSelect: () => onNewQuery(`SELECT *\nFROM ${quoted}\nLIMIT 100;`),
      },
      "separator" as const,
      {
        label: "Copy name",
        icon: <Copy size={14} />,
        onSelect: () => void navigator.clipboard?.writeText(item.name).then(() => push("info", `Copied ${item.name}`)),
      },
      {
        label: "Export CSV",
        icon: <Download size={14} />,
        hidden: item.kind !== "table",
        onSelect: () =>
          void exportTableCSV(databaseId, item.name).catch((err: unknown) =>
            push("error", err instanceof Error ? err.message : "Export failed"),
          ),
      },
    ];
  }

  let index = -1;
  const renderItem = (item: TreeItem): ReactNode => {
    index += 1;
    const i = index;
    const active = activeName === item.name;
    const title = [
      dialect === "postgres" ? `${item.schema}.${item.label}` : item.label,
      item.kind === "view" ? (item.sub === "materialized_view" ? "materialized view" : "view") : null,
      item.rows !== undefined ? `~${item.rows.toLocaleString()} rows` : null,
      item.bytes ? formatBytes(item.bytes) : null,
      item.comment,
    ]
      .filter(Boolean)
      .join(" · ");
    return (
      <button
        key={`${item.kind}:${item.name}`}
        type="button"
        data-index={i}
        className={`dbx-tree-item${active ? " active" : ""}${i === cursor ? " cursor" : ""}${grouped ? " nested" : ""}`}
        title={title}
        onClick={() => open(item, true)}
        onDoubleClick={() => open(item, false)}
        onContextMenu={(e) => ctx.open(e, menuFor(item))}
      >
        {item.kind === "view" ? (
          item.sub === "materialized_view" ? (
            <Layers size={14} className="dbx-tree-icon view" />
          ) : (
            <Eye size={14} className="dbx-tree-icon view" />
          )
        ) : (
          <Table2 size={14} className="dbx-tree-icon" />
        )}
        <span className="truncate" style={{ flex: 1 }}>
          <Highlight text={item.label} query={q} />
        </span>
        {item.rows !== undefined ? <span className="dbx-tree-count">{compactCount(item.rows)}</span> : null}
      </button>
    );
  };

  let body: ReactNode;
  if (isLoading) {
    body = (
      <div className="dbx-tree-note">
        <Spinner /> Connecting…
      </div>
    );
  } else if (error) {
    body = (
      <div className="dbx-tree-note" style={{ color: "var(--danger)" }}>
        {error}
        <button className="btn btn-sm" style={{ marginTop: ".5rem" }} onClick={onRefresh}>
          Try again
        </button>
      </div>
    );
  } else if (items.length === 0) {
    body = <div className="dbx-tree-note">This database has no tables yet.</div>;
  } else if (matches.length === 0) {
    body = <div className="dbx-tree-note">Nothing matches “{query}”.</div>;
  } else if (!grouped) {
    body = matches.map(renderItem);
  } else {
    body = schemas.map((s) => {
      const list = matches.filter((i) => i.schema === s);
      if (list.length === 0) return null;
      const isOpen = Boolean(q) || !collapsed.has(s);
      return (
        <div key={s}>
          <button
            type="button"
            className="dbx-tree-schema"
            aria-expanded={isOpen}
            onClick={() =>
              setCollapsed((prev) => {
                const next = new Set(prev);
                if (next.has(s)) next.delete(s);
                else next.add(s);
                return next;
              })
            }
          >
            <ChevronRight size={13} className={`dbx-chevron${isOpen ? " open" : ""}`} />
            {isOpen ? <FolderOpen size={14} /> : <Folder size={14} />}
            <span className="truncate" style={{ flex: 1 }}>
              {s}
            </span>
            <span className="dbx-tree-count">{list.length}</span>
          </button>
          {isOpen ? list.map(renderItem) : null}
        </div>
      );
    });
  }

  const tableCount = items.filter((i) => i.kind === "table").length;
  const viewCount = items.length - tableCount;

  return (
    <div className="dbx-tree">
      <div className="dbx-tree-head">
        <span className="dbx-tree-title">
          Tables
          {items.length ? (
            <span className="muted" style={{ fontWeight: 400 }}>
              {" "}
              {q ? `${matches.length} of ${items.length}` : `${tableCount}${viewCount ? ` + ${viewCount} views` : ""}`}
            </span>
          ) : null}
        </span>
        <span className="flex items-center">
          <button className="btn btn-ghost btn-icon" onClick={() => onNewQuery()} title="New SQL query" aria-label="New SQL query">
            <SquareTerminal size={14} />
          </button>
          {canWrite ? (
            <button className="btn btn-ghost btn-icon" onClick={onCreateTable} title="Create table" aria-label="Create table">
              <Plus size={15} />
            </button>
          ) : null}
          <button
            className="btn btn-ghost btn-icon"
            onClick={onRefresh}
            title="Refresh the list"
            aria-label="Refresh the list"
            disabled={refreshing}
          >
            <RefreshCw size={14} className={refreshing ? "spin-icon" : undefined} />
          </button>
        </span>
      </div>
      <div className="dbx-search" style={{ margin: "0 .6rem .4rem" }}>
        <Search size={14} />
        <input
          ref={searchRef}
          className="input"
          placeholder="Search tables…"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setCursor(e.target.value ? 0 : -1);
          }}
          onKeyDown={onSearchKey}
          aria-label="Search tables"
        />
        {query ? (
          <button
            className="dbx-search-clear"
            onClick={() => {
              setQuery("");
              setCursor(-1);
              searchRef.current?.focus();
            }}
            aria-label="Clear search"
          >
            <X size={13} />
          </button>
        ) : (
          <kbd className="dbx-search-kbd">Ctrl P</kbd>
        )}
      </div>
      <div className="dbx-tree-list" ref={listRef}>
        {body}
      </div>
      {ctx.element}
    </div>
  );
});
