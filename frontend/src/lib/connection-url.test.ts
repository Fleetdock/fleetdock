import { describe, expect, it } from "vitest";

import { connectionURL } from "./connection-url";

describe("connectionURL", () => {
  it("puts a ClickHouse database in the query string", () => {
    expect(connectionURL("clickhouse", "ch.local", 8443, "analytics", "required")).toBe(
      "https://ch.local:8443/?database=analytics",
    );
    expect(connectionURL("clickhouse", "ch.local", 8123, "a b", "disabled")).toBe("http://ch.local:8123/?database=a%20b");
  });

  it("keeps the other engines' URLs", () => {
    expect(connectionURL("postgresql", "db", 5432, "app")).toBe("postgresql://db:5432/app");
    expect(connectionURL("mariadb", "db", 3306, "app")).toBe("mysql://db:3306/app");
  });
});
