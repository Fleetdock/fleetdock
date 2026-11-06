import type { TLSMode } from "./types";

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
  /** Encryption setting a new connection starts with (default "prefer"). */
  defaultTLS?: TLSMode;
};

export const ENGINES: Record<EngineId, EngineInfo> = {
  postgres: { label: "PostgreSQL", version: "17", versions: ["18", "17", "16", "15", "14"], port: 5432, adminUser: "postgres" },
  mysql: { label: "MySQL", version: "8.4", versions: ["8.4"], port: 3306, adminUser: "root" },
  mariadb: { label: "MariaDB", version: "11.4", versions: ["11.8", "11.4", "10.11", "10.6"], port: 3306, adminUser: "root" },
  clickhouse: {
    label: "ClickHouse",
    version: "25.8",
    versions: ["25.8", "25.3", "24.8"],
    port: 8443,
    adminUser: "default",
    lacks: ["rowEditing", "sqlImport", "backupCompare", "structureEditing", "preview", "insights"],
    portHint: "The HTTP interface: 8443 with TLS, or 8123 without. A server Fleetdock creates serves plain HTTP on the port you pick.",
    defaultTLS: "require",
  },
};

export const ENGINE_IDS = Object.keys(ENGINES) as EngineId[];

/**
 * engineFamily groups engines whose dumps load into each other: an engine id
 * ("mariadb"), or the dump tool a backup records ("pg_dump").
 */
export function engineFamily(engineOrTool: string): "postgres" | "clickhouse" | "mysql" {
  if (engineOrTool === "postgres" || engineOrTool === "pg_dump") return "postgres";
  if (engineOrTool === "clickhouse" || engineOrTool === "clickhouse-native") return "clickhouse";
  return "mysql";
}

/**
 * sendsPasswordInClear reports whether a connection would carry the admin
 * password unencrypted although the setting does not say "never encrypt"
 * outright: ClickHouse's HTTP interface cannot switch to TLS in-band, so
 * "when available" means HTTPS only on 8443 and 443 (as the API decides).
 */
export function sendsPasswordInClear(engine: string, tls: TLSMode, port: number): boolean {
  if (engine !== "clickhouse") return false;
  return tls === "disable" || (tls === "prefer" && port !== 8443 && port !== 443);
}

/** engineLabel returns the display name of an engine id ("postgres" → "PostgreSQL"). */
export function engineLabel(id: string): string {
  return ENGINES[id as EngineId]?.label ?? id;
}

/** engineSupports reports whether an engine has a feature. Unknown engines are assumed to (the API has the final say). */
export function engineSupports(id: string | undefined, feature: EngineFeature): boolean {
  const e = id ? ENGINES[id as EngineId] : undefined;
  return !e?.lacks?.includes(feature);
}
