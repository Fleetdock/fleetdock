/** The database engines Fleetdock manages, with sensible defaults. */
export type EngineId = "mariadb" | "mysql" | "postgres";

export const ENGINES: Record<
  EngineId,
  { label: string; version: string; versions: string[]; port: number; adminUser: string }
> = {
  postgres: { label: "PostgreSQL", version: "17", versions: ["18", "17", "16", "15", "14"], port: 5432, adminUser: "postgres" },
  mysql: { label: "MySQL", version: "8.4", versions: ["8.4"], port: 3306, adminUser: "root" },
  mariadb: { label: "MariaDB", version: "11.4", versions: ["11.8", "11.4", "10.11", "10.6"], port: 3306, adminUser: "root" },
};

export const ENGINE_IDS = Object.keys(ENGINES) as EngineId[];

/**
 * engineFamily groups engines whose dumps load into each other: an engine id
 * ("mariadb"), or the dump tool a backup records ("pg_dump").
 */
export function engineFamily(engineOrTool: string): "postgres" | "mysql" {
  return engineOrTool === "postgres" || engineOrTool === "pg_dump" ? "postgres" : "mysql";
}

/** engineLabel returns the display name of an engine id ("postgres" → "PostgreSQL"). */
export function engineLabel(id: string): string {
  return ENGINES[id as EngineId]?.label ?? id;
}
