// Per-browser conveniences for the Data Browser: open tabs, the last database,
// recent tables, sidebar width. Storage can be missing or throw (private
// windows, blocked site data), so every access is guarded and the page works
// without it.

const PREFIX = "fleetdock:data-browser:";

export function load(key: string): string | null {
  try {
    return window.localStorage.getItem(PREFIX + key);
  } catch {
    return null;
  }
}

export function save(key: string, value: string | null) {
  try {
    if (value === null) window.localStorage.removeItem(PREFIX + key);
    else window.localStorage.setItem(PREFIX + key, value);
  } catch {
    /* storage unavailable */
  }
}

export function loadJSON<T>(key: string, fallback: T, valid: (v: unknown) => v is T): T {
  const raw = load(key);
  if (!raw) return fallback;
  try {
    const v: unknown = JSON.parse(raw);
    return valid(v) ? v : fallback;
  } catch {
    return fallback;
  }
}

const isStringArray = (v: unknown): v is string[] => Array.isArray(v) && v.every((x) => typeof x === "string");

export const workspaceKey = (databaseId: string) => `v1:${databaseId}`;

export function lastDatabase(): string | null {
  return load("last-db");
}

/** rememberDatabase records a database as the last and most recent one. */
export function rememberDatabase(id: string) {
  save("last-db", id);
  pushRecent("recent-dbs", id, 6);
}

export function recentDatabases(): string[] {
  return loadJSON("recent-dbs", [], isStringArray);
}

export type RecentTable = { kind: "table" | "view"; name: string; label: string };

const isRecentTables = (v: unknown): v is RecentTable[] =>
  Array.isArray(v) &&
  v.every(
    (x) =>
      typeof x === "object" &&
      x !== null &&
      ((x as RecentTable).kind === "table" || (x as RecentTable).kind === "view") &&
      typeof (x as RecentTable).name === "string" &&
      typeof (x as RecentTable).label === "string",
  );

export function recentTables(databaseId: string): RecentTable[] {
  return loadJSON(`recent-tables:${databaseId}`, [], isRecentTables);
}

export function rememberTable(databaseId: string, t: RecentTable) {
  const list = recentTables(databaseId).filter((x) => !(x.kind === t.kind && x.name === t.name));
  save(`recent-tables:${databaseId}`, JSON.stringify([t, ...list].slice(0, 8)));
}

function pushRecent(key: string, id: string, max: number) {
  const list = loadJSON(key, [], isStringArray).filter((x) => x !== id);
  save(key, JSON.stringify([id, ...list].slice(0, max)));
}
