import { describe, expect, it } from "vitest";

import type { BrowseColumn } from "../types";
import { cellKind, compactCount, isTruncated, mutationSQL, quoteTable, sqlLiteral, toInsertSQL, toJSON, toTSV } from "./cells";

const col = (name: string, type: string): BrowseColumn => ({ name, type, nullable: true, has_default: false });

describe("cellKind", () => {
  it("classifies PostgreSQL and MySQL types", () => {
    expect(cellKind("bigint")).toBe("number");
    expect(cellKind("numeric(12,2)")).toBe("number");
    expect(cellKind("int(11) unsigned")).toBe("number");
    expect(cellKind("double precision")).toBe("number");
    expect(cellKind("boolean")).toBe("bool");
    expect(cellKind("jsonb")).toBe("json");
    expect(cellKind("timestamp with time zone")).toBe("temporal");
    expect(cellKind("interval")).toBe("temporal");
    expect(cellKind("character varying(255)")).toBe("text");
    expect(cellKind("point")).toBe("text");
  });
});

describe("copying", () => {
  const columns = [col("id", "integer"), col("name", "text"), col("ok", "boolean"), col("doc", "jsonb")];
  const rows = [
    ["1", "it's", "true", '{"a":1}'],
    ["2", "tab\there", null, null],
  ];

  it("writes TSV that survives tabs and quotes", () => {
    expect(toTSV(columns, rows, true)).toBe('id\tname\tok\tdoc\n1\tit\'s\ttrue\t"{""a"":1}"\n2\t"tab\there"\t\t');
  });

  it("writes typed JSON", () => {
    expect(JSON.parse(toJSON(columns, rows))).toEqual([
      { id: 1, name: "it's", ok: true, doc: { a: 1 } },
      { id: 2, name: "tab\there", ok: null, doc: null },
    ]);
  });

  it("keeps numbers that would lose precision as strings", () => {
    expect(JSON.parse(toJSON([col("n", "bigint")], [["9007199254740993"]]))).toEqual([{ n: "9007199254740993" }]);
  });

  it("writes INSERT statements per dialect", () => {
    expect(toInsertSQL(quoteTable("public.t", "postgres", "public"), columns.slice(0, 3), [rows[0].slice(0, 3)], "postgres")).toBe(
      `INSERT INTO "public"."t" ("id", "name", "ok") VALUES (1, 'it''s', TRUE);`,
    );
    expect(sqlLiteral("a\\b", "text", "mysql")).toBe("'a\\\\b'");
    expect(sqlLiteral("a\\b", "text", "postgres")).toBe("'a\\b'");
    expect(sqlLiteral("1; DROP", "number", "postgres")).toBe("'1; DROP'");
    expect(quoteTable("we`ird", "mysql")).toBe("`we``ird`");
  });
});

describe("mutationSQL", () => {
  const columns = [col("id", "integer"), col("name", "text")];
  it("previews each kind of change", () => {
    expect(mutationSQL({ kind: "delete", id: "x", key: { id: "3" } }, '"t"', columns, "postgres")).toBe(
      `DELETE FROM "t" WHERE "id" = 3;`,
    );
    expect(
      mutationSQL({ kind: "update", id: "x", key: { id: "3" }, values: { name: null } }, "`t`", columns, "mysql"),
    ).toBe("UPDATE `t` SET `name` = NULL WHERE `id` = 3;");
    expect(mutationSQL({ kind: "insert", id: "x", values: { name: "o'k" } }, '"t"', columns, "postgres")).toBe(
      `INSERT INTO "t" ("name") VALUES ('o''k');`,
    );
    expect(mutationSQL({ kind: "insert", id: "x", values: {} }, '"t"', columns, "postgres")).toBe(
      `INSERT INTO "t" DEFAULT VALUES;`,
    );
  });
});

describe("misc", () => {
  it("detects truncated cells", () => {
    expect(isTruncated("x".repeat(4096) + "…")).toBe(true);
    expect(isTruncated("short…")).toBe(false);
    expect(isTruncated(null)).toBe(false);
  });

  it("formats compact counts", () => {
    expect(compactCount(950)).toBe("950");
    expect(compactCount(12400)).toBe("12.4k");
    expect(compactCount(3_100_000)).toBe("3.1m");
  });
});
