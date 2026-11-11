"use client";

import { useRouter } from "next/navigation";
import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState, type ReactNode } from "react";

import { useCopy, useToast } from "@/components/toast";
import { Button, IconButton, Input, Spinner } from "@/components/ui";
import { compactCount, quoteTable, type Dialect } from "@/lib/data-browser/cells";
import {
  isSelected,
  loadSelection,
  resolveSelection,
  saveSelection,
  summarizeSchemas,
  type SchemaSelection,
} from "@/lib/data-browser/schemas";
import { collapsedSchemas, saveCollapsedSchemas } from "@/lib/data-browser/storage";
import { objectName, type OpenRequest } from "@/lib/data-browser/tabs";
import { formatBytes } from "@/lib/format";
import { exportTableCSV } from "@/lib/hooks";
import type { DBObject, TableInfo } from "@/lib/types";
import {
  CalendarClock,
  ChevronRight,
  Copy,
  Download,
  Eye,
  Folder,
  FolderOpen,
  FunctionSquare,
  Layers,
  ListOrdered,
  Network,
  Plus,
  RefreshCw,
  Search,
  SquareTerminal,
  Table2,
  TableProperties,
  X,
  Zap,
} from "lucide-react";

import { useContextMenu } from "./context-menu";
import { SchemaPicker } from "./schema-picker";

export type TreeItem = {
  kind: "table" | "view" | "object";
  /** DBObject kind: "materialized_view" for views, function/trigger/… for objects. */
  sub?: string;
  schema: string;
  /** Bare name. */
  label: string;
  /** Identifier the API takes. */
  name: string;
  rows?: number;
  bytes?: number;
  comment?: string;
  /** Table a trigger fires on. */
  table?: string;
};

/** Routines and other non-relation objects, in sidebar order. */
const OBJECT_KINDS: { kind: string; label: string }[] = [
  { kind: "function", label: "Functions" },
  { kind: "procedure", label: "Procedures" },
  { kind: "trigger", label: "Triggers" },
  { kind: "sequence", label: "Sequences" },
  { kind: "event", label: "Events" },
];

export function ObjectIcon({ kind, className }: { kind?: string; className?: string }) {
  const cls = `dbx-tree-icon obj${className ? ` ${className}` : ""}`;
  if (kind === "trigger") return <Zap size={14} className={cls} />;
  if (kind === "sequence") return <ListOrdered size={14} className={cls} />;
  if (kind === "event") return <CalendarClock size={14} className={cls} />;
  return <FunctionSquare size={14} className={cls} />;
}

/**
 * buildItems merges base tables and views into one sorted list, followed by
 * routines, triggers, sequences and events.
 */
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
    if (o.kind === "view" || o.kind === "materialized_view") {
      items.push({ kind: "view", sub: o.kind, schema: o.schema, label: o.name, name: api(o.schema, o.name) });
    } else {
      items.push({ kind: "object", sub: o.kind, schema: o.schema, label: o.name, name: objectName(o), table: o.table });
    }
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
  const router = useRouter();
  const [query, setQuery] = useState("");
  // Folded schema groups are remembered per database.
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set(collapsedSchemas(databaseId)));
  const [collapsedFor, setCollapsedFor] = useState(databaseId);
  if (collapsedFor !== databaseId) {
    setCollapsedFor(databaseId);
    setCollapsed(new Set(collapsedSchemas(databaseId)));
  }
  const [cursor, setCursor] = useState(-1);
  const searchRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const ctx = useContextMenu();
  const { push } = useToast();
  const copy = useCopy();

  useImperativeHandle(ref, () => ({ focusSearch: () => searchRef.current?.select() }), []);

  // Routine groups start collapsed; tables and views are what people open most.
  const [openKinds, setOpenKinds] = useState<Set<string>>(new Set());
  const q = query.trim().toLowerCase();

  // PostgreSQL schemas to show: one remembered choice per database (shared
  // with the ER diagram). Everything else below works on the chosen ones.
  const summary = useMemo(() => summarizeSchemas(items), [items]);
  const [storedSel, setStoredSel] = useState(() => loadSelection(databaseId));
  const [selFor, setSelFor] = useState(databaseId);
  if (selFor !== databaseId) {
    setSelFor(databaseId);
    setStoredSel(loadSelection(databaseId));
  }
  const selection = resolveSelection(storedSel, summary);
  const pickSchemas = summary.length > 1;
  const changeSelection = (sel: SchemaSelection) => {
    setStoredSel(sel);
    saveSelection(databaseId, sel);
    setCursor(-1);
  };
  const scoped = useMemo(
    () => (pickSchemas ? items.filter((i) => isSelected(selection, i.schema)) : items),
    // selection is derived from storedSel and summary.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [items, pickSchemas, storedSel, summary],
  );
  // While searching, matches in the other schemas can be shown on request.
  const [showOthers, setShowOthers] = useState(false);
  const [othersFor, setOthersFor] = useState(q);
  if (othersFor !== q) {
    setOthersFor(q);
    setShowOthers(false);
  }

  const relations = useMemo(() => scoped.filter((i) => i.kind !== "object"), [scoped]);
  const schemas = useMemo(() => [...new Set(scoped.map((i) => i.schema))], [scoped]);
  const grouped = schemas.length > 1;
  const isMatch = (i: TreeItem) => !q || i.label.toLowerCase().includes(q) || `${i.schema}.${i.label}`.toLowerCase().includes(q);
  const matches = relations.filter(isMatch);
  const otherMatches =
    q && pickSchemas
      ? items.filter((i) => i.kind !== "object" && !isSelected(selection, i.schema) && isMatch(i))
      : [];
  const objectGroups = OBJECT_KINDS.map((g) => ({
    ...g,
    list: scoped
      .filter((i) => i.kind === "object" && i.sub === g.kind && isMatch(i))
      .sort((a, b) => a.schema.localeCompare(b.schema) || a.label.localeCompare(b.label)),
  })).filter((g) => g.list.length > 0);
  const objectCount = scoped.length - relations.length;
  // What is on screen, in order: collapsed groups hide their items unless searching.
  const visible = [
    ...(grouped && !q ? matches.filter((i) => !collapsed.has(i.schema)) : matches),
    ...(showOthers ? otherMatches : []),
    ...objectGroups.flatMap((g) => (q || openKinds.has(g.kind) ? g.list : [])),
  ];

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
    if (item.kind === "object") {
      return [
        { label: "Open", icon: <ObjectIcon kind={item.sub} />, onSelect: () => open(item, false) },
        {
          label: `Open table ${item.table}`,
          icon: <Table2 size={14} />,
          hidden: !item.table,
          onSelect: () => {
            const name = dialect === "postgres" && item.table && !item.table.includes(".") ? `${item.schema}.${item.table}` : item.table!;
            onOpen({ kind: "table", name, label: item.table! });
          },
        },
        {
          label: "Copy name",
          icon: <Copy size={14} />,
          onSelect: () => copy(`${item.schema}.${item.label}`, `Copied ${item.label}`),
        },
      ];
    }
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
        onSelect: () => copy(item.name, `Copied ${item.name}`),
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

  // Keyboard position of each item on screen (matches `visible`).
  const position = new Map(visible.map((it, i) => [`${it.kind}:${it.name}`, i]));
  const renderItem = (item: TreeItem): ReactNode => {
    const i = position.get(`${item.kind}:${item.name}`) ?? -1;
    const active = activeName === item.name;
    const title = [
      dialect === "postgres" ? `${item.schema}.${item.label}` : item.label,
      item.kind === "view" ? (item.sub === "materialized_view" ? "materialized view" : "view") : null,
      item.kind === "object" ? item.sub : null,
      item.table ? `on ${item.table}` : null,
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
        className={`dbx-tree-item${active ? " active" : ""}${i === cursor ? " cursor" : ""}${grouped || item.kind === "object" ? " nested" : ""}`}
        title={title}
        // Shift/Ctrl/⌘-click keeps the tab open, like a double click.
        onClick={(e) => open(item, !(e.shiftKey || e.ctrlKey || e.metaKey))}
        onDoubleClick={() => open(item, false)}
        onContextMenu={(e) => ctx.open(e, menuFor(item))}
      >
        {item.kind === "object" ? (
          <ObjectIcon kind={item.sub} />
        ) : item.kind === "view" ? (
          item.sub === "materialized_view" ? (
            <Layers size={14} className="dbx-tree-icon view" />
          ) : (
            <Eye size={14} className="dbx-tree-icon view" />
          )
        ) : (
          <Table2 size={14} className="dbx-tree-icon" />
        )}
        <span className="truncate flex-1">
          {(item.kind === "object" && grouped) || (pickSchemas && !isSelected(selection, item.schema)) ? (
            <span className="muted">{item.schema}.</span>
          ) : null}
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
      <div className="dbx-tree-note text-danger">
        {error}
        <Button size="sm" className="mt-2" onClick={onRefresh}>
          Try again
        </Button>
      </div>
    );
  } else if (relations.length === 0 && objectCount === 0 && !q) {
    body = (
      <div className="dbx-tree-note">
        {items.length === 0 ? "This database has no tables yet." : "No tables in the chosen schemas."}
      </div>
    );
  } else if (matches.length === 0 && objectGroups.length === 0 && otherMatches.length === 0) {
    body = <div className="dbx-tree-note">Nothing matches “{query}”.</div>;
  } else if (matches.length === 0) {
    body = null;
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
                saveCollapsedSchemas(databaseId, [...next]);
                return next;
              })
            }
          >
            <ChevronRight size={13} className={`dbx-chevron${isOpen ? " open" : ""}`} />
            {isOpen ? <FolderOpen size={14} /> : <Folder size={14} />}
            <span className="truncate flex-1">
              {s}
            </span>
            <span className="dbx-tree-count">{list.length}</span>
          </button>
          {isOpen ? list.map(renderItem) : null}
        </div>
      );
    });
  }

  // Matches of the search in schemas that are not shown.
  const othersBody =
    otherMatches.length === 0 || isLoading || error ? null : showOthers ? (
      <div className="dbx-tree-others">
        <div className="dbx-tree-title px-[.45rem] pt-[.7rem] pb-1">In other schemas</div>
        {otherMatches.map((i) => renderItem(i))}
      </div>
    ) : (
      <button type="button" className="dbx-tree-more" onClick={() => setShowOthers(true)}>
        {otherMatches.length} more in other schemas — <span className="link">Show</span>
      </button>
    );

  // Routines, triggers, sequences and events, below the tables.
  const objectsBody =
    isLoading || error || objectGroups.length === 0 ? null : (
      <div className="dbx-tree-objects">
        <div className="dbx-tree-title px-[.45rem] pt-[.7rem] pb-1">
          Routines &amp; more
        </div>
        {objectGroups.map((g) => {
          const isOpen = Boolean(q) || openKinds.has(g.kind);
          return (
            <div key={g.kind}>
              <button
                type="button"
                className="dbx-tree-schema"
                aria-expanded={isOpen}
                onClick={() =>
                  setOpenKinds((prev) => {
                    const next = new Set(prev);
                    if (next.has(g.kind)) next.delete(g.kind);
                    else next.add(g.kind);
                    return next;
                  })
                }
              >
                <ChevronRight size={13} className={`dbx-chevron${isOpen ? " open" : ""}`} />
                <ObjectIcon kind={g.kind} />
                <span className="truncate flex-1">
                  {g.label}
                </span>
                <span className="dbx-tree-count">{g.list.length}</span>
              </button>
              {isOpen ? g.list.map(renderItem) : null}
            </div>
          );
        })}
      </div>
    );

  const tableCount = relations.filter((i) => i.kind === "table").length;
  const viewCount = relations.length - tableCount;

  return (
    <div className="dbx-tree">
      <div className="dbx-tree-head">
        <span className="dbx-tree-title">
          Tables
          {relations.length ? (
            <span className="muted font-normal">
              {" "}
              {q ? `${matches.length} of ${relations.length}` : `${tableCount}${viewCount ? ` + ${viewCount} views` : ""}`}
            </span>
          ) : null}
        </span>
        <span className="flex items-center">
          <IconButton label="New SQL query" onClick={() => onNewQuery()}>
            <SquareTerminal size={14} aria-hidden />
          </IconButton>
          {canWrite ? (
            <IconButton label="Create table" onClick={onCreateTable}>
              <Plus size={15} aria-hidden />
            </IconButton>
          ) : null}
          <IconButton
            label="ER diagram"
            onClick={() => router.push(`/databases/${encodeURIComponent(databaseId)}/diagram`)}
          >
            <Network size={14} aria-hidden />
          </IconButton>
          <IconButton label="Refresh the list" onClick={onRefresh} disabled={refreshing}>
            <RefreshCw size={14} aria-hidden className={refreshing ? "spin-icon" : undefined} />
          </IconButton>
        </span>
      </div>
      {pickSchemas ? (
        <div className="mx-[.6rem] mb-[.4rem]">
          <SchemaPicker summary={summary} selection={selection} onChange={changeSelection} />
        </div>
      ) : null}
      <div className="dbx-search mx-[.6rem] mb-[.4rem]">
        <Search size={14} aria-hidden />
        <Input
          ref={searchRef}
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
        {othersBody}
        {objectsBody}
      </div>
      {ctx.element}
    </div>
  );
});
