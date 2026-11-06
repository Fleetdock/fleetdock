import { describe, expect, it } from "vitest";

import { ENGINE_IDS, engineLabel, engineSupports } from "./engines";

describe("engines", () => {
  it("labels ClickHouse", () => {
    expect(ENGINE_IDS).toContain("clickhouse");
    expect(engineLabel("clickhouse")).toBe("ClickHouse");
  });

  it("knows what ClickHouse lacks", () => {
    expect(engineSupports("clickhouse", "rowEditing")).toBe(false);
    expect(engineSupports("clickhouse", "backups")).toBe(false);
    expect(engineSupports("clickhouse", "provisioning")).toBe(false);
    expect(engineSupports("clickhouse", "structureEditing")).toBe(false);
  });

  it("assumes support for the other engines and for unknown ones", () => {
    for (const id of ["postgres", "mysql", "mariadb", "somethingnew", undefined]) {
      expect(engineSupports(id, "rowEditing")).toBe(true);
      expect(engineSupports(id, "backups")).toBe(true);
    }
  });
});
