import { describe, expect, it } from "vitest";

import { schemaHitHref } from "./schema-search";
import type { SchemaSearchHit } from "./types";

const base: SchemaSearchHit = {
  database_id: "d1",
  database: "shop",
  instance_id: "i1",
  instance: "pg",
  engine: "postgres",
  schema: "sales",
  table: "orders",
  kind: "table",
};

describe("schemaHitHref", () => {
  it("opens a PostgreSQL table qualified by its schema", () => {
    expect(schemaHitHref(base)).toBe("/data?db=d1&table=sales.orders");
  });

  it("opens a MySQL table by bare name and a view as a view", () => {
    expect(schemaHitHref({ ...base, schema: undefined, engine: "mysql", kind: "view", table: "v_orders" })).toBe(
      "/data?db=d1&view=v_orders",
    );
  });

  it("opens a column hit on its table", () => {
    expect(schemaHitHref({ ...base, column: "email", type: "text" })).toBe("/data?db=d1&table=sales.orders");
  });
});
