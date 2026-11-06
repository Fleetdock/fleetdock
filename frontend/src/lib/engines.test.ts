import { describe, expect, it } from "vitest";

import { CLICKHOUSE_DUMP_TOOL, ENGINE_IDS, canRestoreInto, engineLabel, engineSupports } from "./engines";

describe("engines", () => {
  it("labels ClickHouse", () => {
    expect(ENGINE_IDS).toContain("clickhouse");
    expect(engineLabel("clickhouse")).toBe("ClickHouse");
  });

  it("knows what ClickHouse lacks", () => {
    expect(engineSupports("clickhouse", "rowEditing")).toBe(false);
    expect(engineSupports("clickhouse", "backups")).toBe(true);
    expect(engineSupports("clickhouse", "sqlImport")).toBe(false);
    expect(engineSupports("clickhouse", "backupCompare")).toBe(false);
    expect(engineSupports("clickhouse", "provisioning")).toBe(true);
    expect(engineSupports("clickhouse", "structureEditing")).toBe(false);
  });

  it("assumes support for the other engines and for unknown ones", () => {
    for (const id of ["postgres", "mysql", "mariadb", "somethingnew", undefined]) {
      expect(engineSupports(id, "rowEditing")).toBe(true);
      expect(engineSupports(id, "backups")).toBe(true);
    }
  });

  it("restores ClickHouse backups only into ClickHouse", () => {
    expect(canRestoreInto(CLICKHOUSE_DUMP_TOOL, "clickhouse")).toBe(true);
    expect(canRestoreInto(CLICKHOUSE_DUMP_TOOL, "postgres")).toBe(false);
    expect(canRestoreInto("pg_dump", "clickhouse")).toBe(false);
    expect(canRestoreInto("mysqldump", "mariadb")).toBe(true);
    expect(canRestoreInto("clickhouse", "clickhouse")).toBe(true);
  });
});
