"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useLayoutEffect, useMemo, useReducer, useRef, useState, type ReactNode } from "react";

import { useQueryClient } from "@tanstack/react-query";

import { useConfirm } from "@/components/confirm";
import { CreateTableModal } from "@/components/database/create-table-modal";
import { QueryConsole } from "@/components/database/query-console";
import { EmptyState, Spinner } from "@/components/ui";
import { dialectOf, type Dialect } from "@/lib/data-browser/cells";
import {
  lastDatabase,
  load,
  recentTables,
  rememberDatabase,
  rememberTable,
  save,
  workspaceKey,
  type RecentTable,
} from "@/lib/data-browser/storage";
import {
  parseWorkspace,
  reducer,
  serializeWorkspace,
  splitName,
  type OpenRequest,
  type Tab,
} from "@/lib/data-browser/tabs";
import { useCanOn, useDatabase, useDBObjects, useTables } from "@/lib/hooks";
import type { Database } from "@/lib/types";
import { Eye, PanelLeftClose, PanelLeftOpen, SquareTerminal, Table2 } from "lucide-react";

import { DatabaseChooser, DatabasePicker } from "./database-picker";
import { buildItems, ObjectTree, type ObjectTreeHandle } from "./object-tree";
import { TabBar, type CloseKind } from "./tab-bar";
import { TableTab } from "./table-tab";

const MIN_SIDE = 200;
const MAX_SIDE = 520;

function useSidebar() {
  const [width, setWidth] = useState(264);
  const [collapsed, setCollapsed] = useState(false);
  // Restored after mount: storage is not available during server rendering.
  useEffect(() => {
    const w = Number(load("sidebar-width"));
    if (w >= MIN_SIDE && w <= MAX_SIDE) setWidth(w);
    setCollapsed(load("sidebar-collapsed") === "1");
  }, []);
  const toggle = () =>
    setCollapsed((c) => {
      save("sidebar-collapsed", c ? null : "1");
      return !c;
    });
  function startResize(e: React.PointerEvent) {
    e.preventDefault();
    const startX = e.clientX;
    const start = width;
    const target = e.currentTarget as HTMLElement;
    target.setPointerCapture(e.pointerId);
    let w = start;
    const onMove = (ev: PointerEvent) => {
      w = Math.max(MIN_SIDE, Math.min(MAX_SIDE, start + ev.clientX - startX));
      setWidth(w);
    };
    const onUp = () => {
      target.removeEventListener("pointermove", onMove);
      target.removeEventListener("pointerup", onUp);
      save("sidebar-width", String(Math.round(w)));
    };
    target.addEventListener("pointermove", onMove);
    target.addEventListener("pointerup", onUp);
  }
  return { width, collapsed, toggle, startResize };
}

/**
 * DataBrowser is the /data page: a database picker and its tables in a
 * sidebar, and the open tables and queries as tabs.
 */
export function DataBrowser({
  databaseId,
  tableParam,
  viewParam,
}: {
  databaseId: string | null;
  tableParam: string | null;
  viewParam: string | null;
}) {
  const router = useRouter();
  const confirm = useConfirm();
  const side = useSidebar();
  const [drawer, setDrawer] = useState(false);
  const dirtyRef = useRef(0);

  // Without ?db=, reopen the database used last.
  useEffect(() => {
    if (databaseId) return;
    const last = lastDatabase();
    if (last) router.replace(`/data?db=${encodeURIComponent(last)}`);
  }, [databaseId, router]);

  const { data: db, isLoading, error } = useDatabase(databaseId ?? "");
  useEffect(() => {
    if (db) rememberDatabase(db.id);
  }, [db]);

  const pick = useCallback(
    async (id: string) => {
      if (id === databaseId) return;
      if (dirtyRef.current > 0) {
        const ok = await confirm({
          title: "Leave with unsaved changes?",
          message: `${dirtyRef.current} unsaved change${dirtyRef.current === 1 ? "" : "s"} in this database will be lost.`,
          confirmLabel: "Discard and switch",
          danger: true,
        });
        if (!ok) return;
      }
      router.push(`/data?db=${encodeURIComponent(id)}`);
    },
    [confirm, databaseId, router],
  );

  const picker = <DatabasePicker current={db} onPick={(id) => void pick(id)} />;
  const shell = (sidebar: ReactNode, main: ReactNode) => (
    <div className={`dbx${side.collapsed ? " side-collapsed" : ""}`}>
      <h1 className="sr-only">Data browser</h1>
      <div className={`dbx-scrim${drawer ? " open" : ""}`} onClick={() => setDrawer(false)} aria-hidden />
      <aside className={`dbx-side${drawer ? " open" : ""}`} style={{ width: side.width }} aria-label="Database objects">
        <div className="dbx-side-head">{picker}</div>
        {sidebar}
      </aside>
      <div
        className="dbx-resizer"
        onPointerDown={side.startResize}
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize sidebar"
      />
      <section className="dbx-main">{main}</section>
    </div>
  );
  const sideToggle = (
    <>
      <button
        className="btn btn-ghost btn-icon dbx-side-toggle"
        onClick={side.toggle}
        title={side.collapsed ? "Show sidebar" : "Hide sidebar"}
        aria-label={side.collapsed ? "Show sidebar" : "Hide sidebar"}
      >
        {side.collapsed ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}
      </button>
      <button className="btn btn-ghost btn-icon dbx-drawer-toggle" onClick={() => setDrawer(true)} aria-label="Show tables">
        <PanelLeftOpen size={16} />
      </button>
    </>
  );

  if (!databaseId) {
    return shell(null, <DatabaseChooser onPick={(id) => void pick(id)} />);
  }
  if (isLoading) {
    return shell(
      null,
      <div className="dbx-center">
        <Spinner /> Loading…
      </div>,
    );
  }
  if (!db) {
    return shell(
      null,
      <div className="dbx-center">
        <EmptyState
          title="Database not found"
          hint={error ? (error as Error).message : "It may have been removed. Pick another one."}
        />
      </div>,
    );
  }
  if (!db.instance?.has_credentials) {
    return shell(
      null,
      <div className="dbx-center">
        <EmptyState
          title="Fleetdock can't log in to this database server yet"
          hint="Add an admin login on the database server's page to browse its tables."
          action={
            db.instance ? (
              <Link href={`/instances/${db.instance.id}`} className="btn">
                Open {db.instance.name}
              </Link>
            ) : undefined
          }
        />
      </div>,
    );
  }
  return (
    <Workspace
      key={db.id}
      db={db}
      tableParam={tableParam}
      viewParam={viewParam}
      shell={shell}
      sideToggle={sideToggle}
      dirtyRef={dirtyRef}
      closeDrawer={() => setDrawer(false)}
    />
  );
}

function Workspace({
  db,
  tableParam,
  viewParam,
  shell,
  sideToggle,
  dirtyRef,
  closeDrawer,
}: {
  db: Database;
  tableParam: string | null;
  viewParam: string | null;
  shell: (sidebar: ReactNode, main: ReactNode) => ReactNode;
  sideToggle: ReactNode;
  dirtyRef: React.RefObject<number>;
  closeDrawer: () => void;
}) {
  const confirm = useConfirm();
  const qc = useQueryClient();
  const canOn = useCanOn();
  const canWrite = canOn("database:write", { databaseId: db.id, serverId: db.instance?.server_id ?? undefined });
  const dialect: Dialect = dialectOf(db.instance?.engine);
  const pg = dialect === "postgres";

  const [ws, dispatch] = useReducer(reducer, db.id, (id) => parseWorkspace(load(workspaceKey(id))));
  useEffect(() => save(workspaceKey(db.id), serializeWorkspace(ws)), [db.id, ws]);
  const wsRef = useRef(ws);
  useLayoutEffect(() => {
    wsRef.current = ws;
  });
  const activeTab = ws.tabs.find((t) => t.id === ws.active) ?? null;

  // Tabs mount when first shown and then stay mounted (hidden), so switching
  // keeps their scroll position, unsaved edits and query results.
  const [mounted, setMounted] = useState<Set<string>>(() => new Set(ws.active ? [ws.active] : []));
  if (ws.active && !mounted.has(ws.active)) {
    setMounted(new Set(mounted).add(ws.active));
  }

  // ---- unsaved edits per tab ----
  const [dirty, setDirty] = useState<Record<string, number>>({});
  const onDirty = useCallback((id: string, n: number) => {
    setDirty((d) => ((d[id] ?? 0) === n ? d : { ...d, [id]: n }));
  }, []);
  const totalDirty = Object.values(dirty).reduce((a, b) => a + b, 0);
  useEffect(() => {
    dirtyRef.current = totalDirty;
  }, [dirtyRef, totalDirty]);
  useEffect(() => {
    if (!totalDirty) return;
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = ""; // older browsers only prompt when this is set
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [totalDirty]);

  // ---- objects ----
  const tables = useTables(db.id);
  const objects = useDBObjects(db.id);
  const items = useMemo(
    () => buildItems(tables.data?.items ?? [], objects.data ?? [], dialect),
    [tables.data, objects.data, dialect],
  );

  const open = useCallback(
    (req: OpenRequest) => {
      dispatch({ type: "open", tab: req });
      if (!req.preview) rememberTable(db.id, { kind: req.kind, name: req.name, label: req.label });
      closeDrawer();
    },
    [db.id, closeDrawer],
  );
  const newQuery = useCallback((sql?: string) => dispatch({ type: "newQuery", sql }), []);

  // ?table= / ?view= opens (or focuses) that tab; the active tab is written back.
  useEffect(() => {
    const name = tableParam ?? viewParam;
    if (!name) return;
    const kind = tableParam ? "table" : "view";
    const cur = wsRef.current.tabs.find((t) => t.id === wsRef.current.active);
    if (cur && cur.kind === kind && cur.name === name) return;
    dispatch({ type: "open", tab: { kind, name, label: splitName(name, pg).label } });
  }, [tableParam, viewParam, pg]);
  useEffect(() => {
    const q = new URLSearchParams({ db: db.id });
    if (activeTab?.kind === "table") q.set("table", activeTab.name);
    else if (activeTab?.kind === "view") q.set("view", activeTab.name);
    if (new URLSearchParams(window.location.search).toString() !== q.toString()) {
      window.history.replaceState(null, "", `/data?${q.toString()}`);
    }
  }, [db.id, activeTab?.kind, activeTab?.name]);

  const close = useCallback(
    async (kind: CloseKind, id: string) => {
      const tabs = wsRef.current.tabs;
      const i = tabs.findIndex((t) => t.id === id);
      const closing =
        kind === "close"
          ? tabs.filter((t) => t.id === id)
          : kind === "closeOthers"
            ? tabs.filter((t) => t.id !== id)
            : kind === "closeRight"
              ? tabs.slice(i + 1)
              : tabs;
      const unsaved = closing.filter((t) => (dirty[t.id] ?? 0) > 0);
      if (unsaved.length) {
        const n = unsaved.reduce((s, t) => s + (dirty[t.id] ?? 0), 0);
        const ok = await confirm({
          title: "Close with unsaved changes?",
          message: `${n} unsaved change${n === 1 ? "" : "s"} in ${unsaved.map((t) => t.label).join(", ")} will be lost.`,
          confirmLabel: "Discard and close",
          danger: true,
        });
        if (!ok) return;
      }
      dispatch(kind === "closeAll" ? { type: "closeAll" } : { type: kind, id });
      setMounted((m) => {
        const next = new Set(m);
        for (const t of closing) next.delete(t.id);
        return next;
      });
    },
    [confirm, dirty],
  );

  // ---- keyboard ----
  const tree = useRef<ObjectTreeHandle>(null);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === "p") {
        e.preventDefault();
        tree.current?.focusSearch();
        return;
      }
      if (!e.altKey || e.ctrlKey || e.metaKey || document.querySelector('[role="dialog"]')) return;
      // Alt+letter types characters on macOS; leave text fields alone.
      const el = e.target as HTMLElement | null;
      if (el && (el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName))) return;
      const cur = wsRef.current;
      if (e.code === "KeyW") {
        if (cur.active) {
          e.preventDefault();
          void close("close", cur.active);
        }
      } else if (e.code === "KeyN") {
        e.preventDefault();
        newQuery();
      } else if (e.code === "BracketRight" || e.code === "BracketLeft") {
        e.preventDefault();
        dispatch({ type: "cycle", delta: e.code === "BracketRight" ? 1 : -1 });
      } else if (/^Digit[1-9]$/.test(e.code)) {
        const t = cur.tabs[e.code === "Digit9" ? cur.tabs.length - 1 : Number(e.code.slice(5)) - 1];
        if (t) {
          e.preventDefault();
          dispatch({ type: "activate", id: t.id });
        }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [close, newQuery]);

  const [creating, setCreating] = useState(false);
  const refreshObjects = () => {
    void qc.invalidateQueries({ queryKey: ["tables", db.id] });
    void qc.invalidateQueries({ queryKey: ["db-objects", db.id] });
  };
  const listError = tables.error ?? objects.error;

  const sidebar = (
    <ObjectTree
      ref={tree}
      databaseId={db.id}
      items={items}
      isLoading={tables.isLoading}
      error={listError ? (listError as Error).message : undefined}
      dialect={dialect}
      activeName={activeTab && activeTab.kind !== "query" ? activeTab.name : null}
      canWrite={canWrite && !db.system}
      onOpen={open}
      onNewQuery={newQuery}
      onCreateTable={() => setCreating(true)}
      onRefresh={refreshObjects}
      refreshing={tables.isFetching || objects.isFetching}
    />
  );

  const main = (
    <>
      <TabBar
        tabs={ws.tabs}
        active={ws.active}
        dirty={dirty}
        onActivate={(id) => dispatch({ type: "activate", id })}
        onPin={(id) => dispatch({ type: "pin", id })}
        onClose={(kind, id) => void close(kind, id)}
        onMove={(id, to) => dispatch({ type: "move", id, to })}
        onNewQuery={() => newQuery()}
        leading={sideToggle}
      />
      <div className="dbx-panes">
        {ws.tabs.length === 0 ? (
          <Welcome db={db} items={items.length} onOpen={open} onNewQuery={newQuery} />
        ) : null}
        {ws.tabs.map((t) =>
          mounted.has(t.id) ? (
            <div
              key={t.id}
              id={`pane-${t.id}`}
              role="tabpanel"
              className="dbx-panel"
              hidden={t.id !== ws.active}
            >
              <TabContent
                tab={t}
                active={t.id === ws.active}
                db={db}
                canWrite={canWrite}
                dialect={dialect}
                dispatch={dispatch}
                onDirty={onDirty}
                onOpen={open}
                onNewQuery={newQuery}
              />
            </div>
          ) : null,
        )}
      </div>
      {creating ? (
        <CreateTableModal
          databaseId={db.id}
          schema={pg ? "public" : null}
          onClose={() => setCreating(false)}
          onCreated={(name) => {
            setCreating(false);
            refreshObjects();
            const full = pg && !name.includes(".") ? `public.${name}` : name;
            open({ kind: "table", name: full, label: splitName(full, pg).label });
          }}
        />
      ) : null}
    </>
  );

  return shell(sidebar, main);
}

function TabContent({
  tab,
  active,
  db,
  canWrite,
  dialect,
  dispatch,
  onDirty,
  onOpen,
  onNewQuery,
}: {
  tab: Tab;
  active: boolean;
  db: Database;
  canWrite: boolean;
  dialect: Dialect;
  dispatch: React.Dispatch<import("@/lib/data-browser/tabs").Action>;
  onDirty: (id: string, n: number) => void;
  onOpen: (req: OpenRequest) => void;
  onNewQuery: (sql?: string) => void;
}) {
  if (tab.kind === "query") {
    return (
      <div className="dbx-query">
        <QueryConsole databaseId={db.id} canWrite={canWrite} initialSql={tab.sql} />
      </div>
    );
  }
  return (
    <TableTab
      databaseId={db.id}
      tab={tab}
      active={active}
      canWrite={canWrite}
      dialect={dialect}
      dispatch={dispatch}
      onDirty={onDirty}
      onOpen={onOpen}
      onNewQuery={onNewQuery}
    />
  );
}

/** Welcome fills the main area while no tab is open. */
function Welcome({
  db,
  items,
  onOpen,
  onNewQuery,
}: {
  db: Database;
  items: number;
  onOpen: (req: OpenRequest) => void;
  onNewQuery: (sql?: string) => void;
}) {
  const [recent, setRecent] = useState<RecentTable[]>([]);
  useEffect(() => setRecent(recentTables(db.id)), [db.id]);
  const shortcuts: [string, string][] = [
    ["Ctrl P", "Find a table"],
    ["Alt N", "New SQL query"],
    ["Alt [ / ]", "Previous / next tab"],
    ["Alt 1…9", "Go to tab"],
    ["Alt W", "Close tab"],
    ["Ctrl S", "Save changes"],
  ];
  return (
    <div className="dbx-welcome">
      <div className="dbx-welcome-icon">
        <Table2 size={22} />
      </div>
      <h2 className="font-semibold" style={{ fontSize: "1.15rem", margin: ".2rem 0" }}>
        {db.name}
      </h2>
      <p className="muted text-sm" style={{ margin: 0, maxWidth: "32rem" }}>
        {items
          ? "Pick a table in the sidebar. A single click previews it; a double click keeps it open in its own tab."
          : "Tables you create appear in the sidebar."}
      </p>
      <div className="flex gap-2" style={{ marginTop: "1rem" }}>
        <button className="btn btn-sm" onClick={() => onNewQuery()}>
          <SquareTerminal size={14} /> New SQL query
        </button>
      </div>
      {recent.length ? (
        <div className="dbx-welcome-recent">
          <div className="dbx-welcome-label">Recently opened</div>
          {recent.map((t) => (
            <button key={`${t.kind}:${t.name}`} className="dbx-welcome-item" onClick={() => onOpen({ ...t })}>
              {t.kind === "view" ? <Eye size={14} className="muted" /> : <Table2 size={14} className="muted" />}
              <span className="truncate">{t.name}</span>
            </button>
          ))}
        </div>
      ) : null}
      <div className="dbx-welcome-keys">
        {shortcuts.map(([k, label]) => (
          <div key={k}>
            <kbd>{k}</kbd> <span className="muted">{label}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
