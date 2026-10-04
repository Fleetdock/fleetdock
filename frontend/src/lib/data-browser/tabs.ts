import type { RowFilter, SortKey } from "../types";

// The Data Browser keeps one workspace per database: the open tabs, which one
// is active, and each tab's view state (filters, sort, page, column widths).
// It is plain data so it can be persisted to localStorage and unit-tested; the
// components only dispatch actions to the reducer below.

export type TabKind = "table" | "view" | "query";

export const PAGE_SIZES = [50, 100, 200, 500] as const;
export const DEFAULT_PAGE_SIZE = 100;

/** TableView is the per-tab state of a table or view tab. */
export interface TableView {
  mode: "data" | "structure";
  search: string;
  filters: RowFilter[];
  sort: SortKey[];
  page: number;
  pageSize: number;
  /** Column widths in px, by column name. */
  widths: Record<string, number>;
}

export interface Tab {
  /** Stable identity: "table:<name>", "view:<name>" or "query:<n>". */
  id: string;
  kind: TabKind;
  /**
   * What the API addresses: the table identifier for table/view tabs
   * ("schema.table" on PostgreSQL, the bare name on MySQL/MariaDB), or the
   * title for query tabs.
   */
  name: string;
  /** Short label shown on the tab. */
  label: string;
  /** A preview tab is replaced by the next single-click open (shown in italics). */
  preview: boolean;
  view: TableView;
  /** Initial SQL of a query tab. */
  sql?: string;
}

export interface Workspace {
  tabs: Tab[];
  active: string | null;
  /** Counter for naming query tabs. */
  seq: number;
}

export const emptyWorkspace: Workspace = { tabs: [], active: null, seq: 0 };

export function defaultView(): TableView {
  return { mode: "data", search: "", filters: [], sort: [], page: 1, pageSize: DEFAULT_PAGE_SIZE, widths: {} };
}

export const tabId = (kind: TabKind, name: string) => `${kind}:${name}`;

/**
 * splitName splits an API table identifier into schema and bare name. On
 * PostgreSQL the identifier is "schema.table"; MySQL/MariaDB use the bare name.
 */
export function splitName(name: string, postgres: boolean): { schema?: string; label: string } {
  const i = postgres ? name.indexOf(".") : -1;
  return i > 0 ? { schema: name.slice(0, i), label: name.slice(i + 1) } : { label: name };
}

export type OpenRequest = {
  kind: "table" | "view";
  name: string;
  label: string;
  preview?: boolean;
  /** View state to apply (e.g. a filter when following a foreign key). */
  view?: Partial<TableView>;
};

export type Action =
  | { type: "open"; tab: OpenRequest }
  | { type: "newQuery"; sql?: string; title?: string }
  | { type: "activate"; id: string }
  | { type: "pin"; id: string }
  | { type: "close"; id: string }
  | { type: "closeOthers"; id: string }
  | { type: "closeRight"; id: string }
  | { type: "closeAll" }
  | { type: "move"; id: string; to: number }
  | { type: "cycle"; delta: number }
  | { type: "rename"; id: string; name: string; label: string }
  | { type: "update"; id: string; view: Partial<TableView>; pin?: boolean };

/** neighbour picks the tab to activate when `closing` disappears. */
function neighbour(tabs: Tab[], closing: Set<string>, active: string | null): string | null {
  if (active && !closing.has(active)) return active;
  const i = tabs.findIndex((t) => t.id === active);
  const rest = tabs.filter((t) => !closing.has(t.id));
  if (rest.length === 0) return null;
  // Prefer the first surviving tab to the right, else the closest on the left.
  for (let j = i + 1; j < tabs.length; j++) if (!closing.has(tabs[j].id)) return tabs[j].id;
  for (let j = i - 1; j >= 0; j--) if (!closing.has(tabs[j].id)) return tabs[j].id;
  return rest[0].id;
}

function without(ws: Workspace, ids: Set<string>): Workspace {
  if (ids.size === 0) return ws;
  return { ...ws, tabs: ws.tabs.filter((t) => !ids.has(t.id)), active: neighbour(ws.tabs, ids, ws.active) };
}

export function reducer(ws: Workspace, a: Action): Workspace {
  switch (a.type) {
    case "open": {
      const id = tabId(a.tab.kind, a.tab.name);
      const existing = ws.tabs.find((t) => t.id === id);
      if (existing) {
        const tabs = ws.tabs.map((t) =>
          t.id === id
            ? {
                ...t,
                preview: t.preview && Boolean(a.tab.preview),
                view: a.tab.view ? { ...t.view, ...a.tab.view, page: a.tab.view.page ?? 1 } : t.view,
              }
            : t,
        );
        return { ...ws, tabs, active: id };
      }
      const tab: Tab = {
        id,
        kind: a.tab.kind,
        name: a.tab.name,
        label: a.tab.label,
        preview: Boolean(a.tab.preview),
        view: { ...defaultView(), ...a.tab.view },
      };
      // A preview open replaces the current preview tab in place.
      const previewAt = tab.preview ? ws.tabs.findIndex((t) => t.preview) : -1;
      if (previewAt >= 0) {
        const tabs = ws.tabs.slice();
        tabs[previewAt] = tab;
        return { ...ws, tabs, active: id };
      }
      const at = ws.tabs.findIndex((t) => t.id === ws.active);
      const tabs = ws.tabs.slice();
      tabs.splice(at < 0 ? tabs.length : at + 1, 0, tab);
      return { ...ws, tabs, active: id };
    }
    case "newQuery": {
      const seq = ws.seq + 1;
      const tab: Tab = {
        id: `query:${seq}`,
        kind: "query",
        name: a.title ?? `Query ${seq}`,
        label: a.title ?? `Query ${seq}`,
        preview: false,
        view: defaultView(),
        sql: a.sql,
      };
      const at = ws.tabs.findIndex((t) => t.id === ws.active);
      const tabs = ws.tabs.slice();
      tabs.splice(at < 0 ? tabs.length : at + 1, 0, tab);
      return { tabs, active: tab.id, seq };
    }
    case "activate":
      return ws.tabs.some((t) => t.id === a.id) ? { ...ws, active: a.id } : ws;
    case "pin":
      return {
        ...ws,
        tabs: ws.tabs.map((t) => (t.id === a.id && t.preview ? { ...t, preview: false } : t)),
      };
    case "close":
      return without(ws, new Set([a.id]));
    case "closeOthers":
      return { ...without(ws, new Set(ws.tabs.filter((t) => t.id !== a.id).map((t) => t.id))), active: a.id };
    case "closeRight": {
      const i = ws.tabs.findIndex((t) => t.id === a.id);
      if (i < 0) return ws;
      const ids = new Set(ws.tabs.slice(i + 1).map((t) => t.id));
      const next = without(ws, ids);
      return ids.has(ws.active ?? "") ? { ...next, active: a.id } : next;
    }
    case "closeAll":
      return { ...ws, tabs: [], active: null };
    case "move": {
      const from = ws.tabs.findIndex((t) => t.id === a.id);
      if (from < 0) return ws;
      const tabs = ws.tabs.slice();
      const [tab] = tabs.splice(from, 1);
      const to = Math.max(0, Math.min(tabs.length, a.to));
      tabs.splice(to, 0, tab);
      return { ...ws, tabs };
    }
    case "cycle": {
      if (ws.tabs.length === 0) return ws;
      const i = Math.max(0, ws.tabs.findIndex((t) => t.id === ws.active));
      const n = ws.tabs.length;
      return { ...ws, active: ws.tabs[(((i + a.delta) % n) + n) % n].id };
    }
    case "rename": {
      const tab = ws.tabs.find((t) => t.id === a.id);
      if (!tab || tab.kind === "query") return ws;
      const id = tabId(tab.kind, a.name);
      if (id !== a.id && ws.tabs.some((t) => t.id === id)) {
        // The new name is already open in another tab: keep that one.
        return { ...without(ws, new Set([a.id])), active: id };
      }
      return {
        ...ws,
        tabs: ws.tabs.map((t) => (t.id === a.id ? { ...t, id, name: a.name, label: a.label } : t)),
        active: ws.active === a.id ? id : ws.active,
      };
    }
    case "update":
      return {
        ...ws,
        tabs: ws.tabs.map((t) =>
          t.id === a.id ? { ...t, preview: a.pin ? false : t.preview, view: { ...t.view, ...a.view } } : t,
        ),
      };
  }
}

// ---- persistence ----

const isObj = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const FILTER_OPS = new Set(["eq", "ne", "lt", "lte", "gt", "gte", "contains", "starts_with", "is_null", "not_null"]);

function parseView(v: unknown): TableView {
  const d = defaultView();
  if (!isObj(v)) return d;
  const filters = Array.isArray(v.filters)
    ? v.filters.filter(
        (f): f is RowFilter =>
          isObj(f) &&
          typeof f.column === "string" &&
          typeof f.op === "string" &&
          FILTER_OPS.has(f.op) &&
          (f.value === undefined || f.value === null || typeof f.value === "string"),
      )
    : [];
  const sort = Array.isArray(v.sort)
    ? v.sort.filter((s): s is SortKey => isObj(s) && typeof s.column === "string" && typeof s.desc === "boolean")
    : [];
  const widths: Record<string, number> = {};
  if (isObj(v.widths)) {
    for (const [k, w] of Object.entries(v.widths)) {
      if (typeof w === "number" && Number.isFinite(w)) widths[k] = Math.max(40, Math.min(2000, Math.round(w)));
    }
  }
  return {
    mode: v.mode === "structure" ? "structure" : "data",
    search: typeof v.search === "string" ? v.search : "",
    filters,
    sort,
    page: typeof v.page === "number" && v.page >= 1 ? Math.floor(v.page) : 1,
    pageSize: PAGE_SIZES.includes(v.pageSize as (typeof PAGE_SIZES)[number]) ? (v.pageSize as number) : d.pageSize,
    widths,
  };
}

/** parseWorkspace restores a persisted workspace, dropping anything malformed. */
export function parseWorkspace(raw: string | null): Workspace {
  if (!raw) return emptyWorkspace;
  let v: unknown;
  try {
    v = JSON.parse(raw);
  } catch {
    return emptyWorkspace;
  }
  if (!isObj(v) || !Array.isArray(v.tabs)) return emptyWorkspace;
  const seen = new Set<string>();
  const tabs: Tab[] = [];
  for (const t of v.tabs) {
    if (!isObj(t) || typeof t.name !== "string" || typeof t.label !== "string") continue;
    if (t.kind !== "table" && t.kind !== "view" && t.kind !== "query") continue;
    const id = t.kind === "query" ? (typeof t.id === "string" && t.id.startsWith("query:") ? t.id : null) : tabId(t.kind, t.name);
    if (!id || seen.has(id)) continue;
    seen.add(id);
    tabs.push({
      id,
      kind: t.kind,
      name: t.name,
      label: t.label,
      preview: t.preview === true,
      view: parseView(t.view),
      sql: typeof t.sql === "string" ? t.sql : undefined,
    });
  }
  const active = typeof v.active === "string" && seen.has(v.active) ? v.active : (tabs[0]?.id ?? null);
  const seq = typeof v.seq === "number" && v.seq >= 0 ? Math.floor(v.seq) : 0;
  return { tabs, active, seq };
}

export function serializeWorkspace(ws: Workspace): string {
  return JSON.stringify(ws);
}
