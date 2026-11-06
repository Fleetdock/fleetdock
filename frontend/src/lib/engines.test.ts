import { describe, expect, it } from "vitest";

import { ENGINE_IDS, ENGINES, engineFamily, engineLabel, engineSupports, sendsPasswordInClear } from "./engines";

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

  it("gives ClickHouse its own family, matching its backups", () => {
    expect(engineFamily("clickhouse")).toBe("clickhouse");
    expect(engineFamily("clickhouse-native")).toBe("clickhouse");
    expect(engineFamily("mariadb")).toBe(engineFamily("mysql"));
    expect(engineFamily("pg_dump")).toBe("postgres");
  });

  it("starts ClickHouse encrypted on its HTTPS port", () => {
    expect(ENGINES.clickhouse.port).toBe(8443);
    expect(ENGINES.clickhouse.defaultTLS).toBe("require");
  });

  it("flags settings that send a ClickHouse password over plain HTTP", () => {
    expect(sendsPasswordInClear("clickhouse", "prefer", 8123)).toBe(true);
    expect(sendsPasswordInClear("clickhouse", "disable", 8443)).toBe(true);
    expect(sendsPasswordInClear("clickhouse", "prefer", 8443)).toBe(false);
    expect(sendsPasswordInClear("clickhouse", "require", 8123)).toBe(false);
    // MySQL and PostgreSQL negotiate TLS in-band; "prefer" is fine there.
    expect(sendsPasswordInClear("postgres", "prefer", 5432)).toBe(false);
  });
});
