/** The database engines Fleetdock manages, with sensible defaults. */
export type EngineId = "mariadb" | "mysql" | "postgres" | "clickhouse";

/** Features not every engine has. The API refuses them too; these flags only keep the dashboard from offering them. */
export type EngineFeature =
  /** Insert, edit and delete rows in the data browser (and undo, CSV import). */
  | "rowEditing"
  /** Backups, restores, scheduled backups and moves. */
  | "backups"
  /** Load a .sql file into a database. */
  | "sqlImport"
  /** Compare a table row by row with a restored backup copy. */
  | "backupCompare"
  /** Fleetdock creates the engine as a container on a connected server. */
  | "provisioning"
  /** Create, alter, rename, truncate and drop tables and indexes from the dashboard. */
  | "structureEditing"
  /** Run a write script and roll it back to see what it would change. */
  | "preview"
  /** Query plans (Explain / Analyze), top queries and index health. */
  | "insights";

type EngineInfo = {
  label: string;
  version: string;
  versions: string[];
  port: number;
  adminUser: string;
  /** Features this engine lacks; every other feature is supported. */
  lacks?: EngineFeature[];
  /** Shown under the port field when the default needs explaining. */
  portHint?: string;
};

export const ENGINES: Record<EngineId, EngineInfo> = {
  postgres: { label: "PostgreSQL", version: "16", versions: ["17", "16", "15", "14"], port: 5432, adminUser: "postgres" },
  mysql: { label: "MySQL", version: "8.4", versions: ["8.4", "8.0"], port: 3306, adminUser: "root" },
  mariadb: { label: "MariaDB", version: "11.4", versions: ["11.4", "10.11", "10.6"], port: 3306, adminUser: "root" },
  clickhouse: {
    label: "ClickHouse",
    version: "25.8",
    versions: ["25.8", "25.3", "24.8"],
    port: 8123,
    adminUser: "default",
    lacks: ["rowEditing", "sqlImport", "backupCompare", "structureEditing", "preview", "insights"],
    portHint: "The HTTP interface: 8123, or 8443 with TLS. A server Fleetdock creates serves HTTP on the port you pick.",
  },
};

export const ENGINE_IDS = Object.keys(ENGINES) as EngineId[];

/** engineLabel returns the display name of an engine id ("postgres" → "PostgreSQL"). */
export function engineLabel(id: string): string {
  return ENGINES[id as EngineId]?.label ?? id;
}

/** engineSupports reports whether an engine has a feature. Unknown engines are assumed to (the API has the final say). */
export function engineSupports(id: string | undefined, feature: EngineFeature): boolean {
  const e = id ? ENGINES[id as EngineId] : undefined;
  return !e?.lacks?.includes(feature);
}

/** The dump tool of a ClickHouse backup (backups.engine). */
export const CLICKHOUSE_DUMP_TOOL = "clickhouse-native";

/**
 * canRestoreInto reports whether data from a source engine can be restored
 * into a target engine: ClickHouse backups are not SQL dumps, so they only go
 * into ClickHouse, and SQL dumps never do. Pass the backup's tool or the
 * source database's engine as source.
 */
export function canRestoreInto(source: string | undefined, target: string | undefined): boolean {
  const ch = (e: string | undefined) => e === "clickhouse" || e === CLICKHOUSE_DUMP_TOOL;
  return ch(source) === ch(target);
}
