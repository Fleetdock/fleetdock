import { describe, expect, it } from "vitest";

import type { Diagram, DiagramTable } from "../../../lib/types";
import {
  MAX_CARD_COLUMNS,
  buildGraph,
  cardColumns,
  cardSize,
  isSavedPositions,
  mostConnected,
  neighbourhood,
  relationLabel,
  schemasOf,
  tableId,
  usableSaved,
} from "./graph";

const col = (name: string, extra: Partial<{ nullable: boolean; primary_key: boolean }> = {}) => ({
  name,
  type: "int",
  nullable: false,
  ...extra,
});

const diagram: Diagram = {
  truncated: false,
  tables: [
    { name: "customers", kind: "table", columns: [col("id", { primary_key: true })] },
    {
      name: "orders",
      kind: "table",
      columns: [col("id", { primary_key: true }), col("customer_id"), col("coupon_id", { nullable: true })],
    },
    { name: "coupons", kind: "table", columns: [col("id", { primary_key: true })] },
    { name: "items", kind: "table", columns: [col("order_id")] },
    { name: "staff", kind: "table", columns: [col("id"), col("boss", { nullable: true })] },
    { name: "open_orders", kind: "view", columns: [col("id")] },
  ],
  relations: [
    { name: "fk_cust", table: "orders", columns: ["customer_id"], ref_table: "customers", ref_columns: ["id"], on_delete: "CASCADE", on_update: "NO ACTION" },
    { name: "fk_coupon", table: "orders", columns: ["coupon_id"], ref_table: "coupons", ref_columns: ["id"], on_delete: "SET NULL", on_update: "" },
    { name: "fk_order", table: "items", columns: ["order_id"], ref_table: "orders", ref_columns: ["id"], on_delete: "", on_update: "" },
    { name: "fk_boss", table: "staff", columns: ["boss"], ref_table: "staff", ref_columns: ["id"], on_delete: "", on_update: "" },
    // Points at a table the login cannot see: dropped.
    { name: "fk_hidden", table: "items", columns: ["order_id"], ref_table: "secret", ref_columns: ["id"], on_delete: "", on_update: "" },
  ],
};

describe("buildGraph", () => {
  const g = buildGraph(diagram);

  it("keeps relations between listed tables only", () => {
    expect(g.relations.map((r) => r.name)).toEqual(["fk_cust", "fk_coupon", "fk_order", "fk_boss"]);
  });

  it("marks optional and self relations", () => {
    const by = Object.fromEntries(g.relations.map((r) => [r.name, r]));
    expect(by.fk_cust.optional).toBe(false);
    expect(by.fk_coupon.optional).toBe(true);
    expect(by.fk_boss.self).toBe(true);
  });

  it("records key columns on both sides and the degree", () => {
    expect([...g.keyColumns.get("orders")!].sort()).toEqual(["coupon_id", "customer_id", "id"]);
    expect(g.degree.get("orders")).toBe(3);
    expect(g.degree.get("staff")).toBe(1);
    expect(mostConnected(g)).toBe("orders");
  });

  it("walks neighbours in both directions up to a depth", () => {
    expect([...neighbourhood(g, ["items"], 1)].sort()).toEqual(["items", "orders"]);
    expect([...neighbourhood(g, ["items"], 2)].sort()).toEqual(["coupons", "customers", "items", "orders"]);
    expect([...neighbourhood(g, ["nope"], 2)]).toEqual([]);
  });

  it("describes a relation", () => {
    expect(relationLabel(g.relations[1])).toBe("orders(coupon_id) → coupons(id), optional, on delete set null");
  });
});

describe("tables", () => {
  it("names PostgreSQL tables with their schema", () => {
    expect(tableId({ schema: "billing", name: "invoices" })).toBe("billing.invoices");
    expect(tableId({ name: "invoices" })).toBe("invoices");
    expect(
      schemasOf(
        buildGraph({
          truncated: false,
          relations: [],
          tables: [
            { schema: "public", name: "a", kind: "table", columns: [] },
            { schema: "billing", name: "b", kind: "table", columns: [] },
          ],
        }),
      ),
    ).toEqual(["billing", "public"]);
  });

  it("folds long column lists but keeps key columns", () => {
    const columns = Array.from({ length: MAX_CARD_COLUMNS + 10 }, (_, i) => col(`c${i}`));
    const t: DiagramTable = { name: "wide", kind: "table", columns };
    const last = `c${MAX_CARD_COLUMNS + 9}`;
    const cols = cardColumns(t, new Set([last]));
    expect(cols.shown).toHaveLength(MAX_CARD_COLUMNS + 1);
    expect(cols.shown.at(-1)!.name).toBe(last);
    expect(cols.hidden).toBe(9);
    const size = cardSize(t, cols);
    const small = cardSize(t, cardColumns({ ...t, columns: columns.slice(0, 2) }, undefined));
    expect(size.height).toBeGreaterThan(small.height);
    expect(small.width).toBeGreaterThanOrEqual(200);
  });
});

describe("saved positions", () => {
  it("validates and checks coverage", () => {
    expect(isSavedPositions({ a: { x: 1, y: 2 } })).toBe(true);
    expect(isSavedPositions({ a: { x: "1", y: 2 } })).toBe(false);
    expect(isSavedPositions([])).toBe(false);
    expect(usableSaved({ a: { x: 0, y: 0 } }, ["a"])).toBe(true);
    expect(usableSaved({ a: { x: 0, y: 0 } }, ["a", "b"])).toBe(false);
    expect(usableSaved({}, [])).toBe(false);
  });
});

describe("ghost tables", () => {
  const d: Diagram = {
    truncated: false,
    tables: [
      { schema: "billing", name: "invoices", kind: "table", columns: [col("id", { primary_key: true }), col("company_id")] },
      { schema: "billing", name: "lines", kind: "table", columns: [col("invoice_id")] },
    ],
    relations: [
      { name: "fk_co", table: "billing.invoices", columns: ["company_id"], ref_table: "crm.companies", ref_columns: ["id"], on_delete: "", on_update: "" },
      { name: "fk_inv", table: "billing.lines", columns: ["invoice_id"], ref_table: "billing.invoices", ref_columns: ["id"], on_delete: "", on_update: "" },
      // A table of a shown schema the login cannot read: no ghost, the relation is dropped.
      { name: "fk_hidden", table: "billing.lines", columns: ["invoice_id"], ref_table: "billing.secret", ref_columns: ["id"], on_delete: "", on_update: "" },
    ],
  };

  it("draws tables of other schemas as ghosts", () => {
    const g = buildGraph(d, new Set(["billing"]));
    expect(g.tables.get("crm.companies")).toMatchObject({ ghost: true, schema: "crm", name: "companies", columns: [] });
    expect(g.tables.has("billing.secret")).toBe(false);
    expect(g.relations.map((r) => r.name)).toEqual(["fk_co", "fk_inv"]);
    expect(schemasOf(g)).toEqual(["billing"]);
    expect(mostConnected(g)).toBe("billing.invoices");
  });

  it("draws no ghosts without a schema filter", () => {
    expect(buildGraph(d).tables.has("crm.companies")).toBe(false);
  });
});
