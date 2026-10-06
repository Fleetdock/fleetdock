import { describe, expect, it } from "vitest";

import { isReadOnly, leadingKeyword, splitStatements, writeStatements } from "./sql";

// These cases mirror backend/internal/platform/engine/sqlsplit_test.go and
// mariadb_admin_test.go: the client warning must classify exactly like the
// server that enforces it.
describe("splitStatements", () => {
  const cases: [string, string, string[]][] = [
    ["single", "SELECT 1", ["SELECT 1"]],
    ["two", "SELECT 1; SELECT 2;", ["SELECT 1", "SELECT 2"]],
    ["semicolon in string", "SELECT 'a;b'; SELECT 2", ["SELECT 'a;b'", "SELECT 2"]],
    ["doubled quote", "SELECT 'it''s; ok'; SELECT 3", ["SELECT 'it''s; ok'", "SELECT 3"]],
    ["backslash escape", "SELECT 'a\\';b'; SELECT 4", ["SELECT 'a\\';b'", "SELECT 4"]],
    ["quoted identifiers", 'SELECT "a;b" FROM `t;x`; SELECT 5', ['SELECT "a;b" FROM `t;x`', "SELECT 5"]],
    ["line comment", "SELECT 1; -- drop; this\nSELECT 2", ["SELECT 1", "-- drop; this\nSELECT 2"]],
    ["block comment", "SELECT /* ; */ 1; SELECT 2", ["SELECT /* ; */ 1", "SELECT 2"]],
    ["comment only dropped", "SELECT 1; -- trailing", ["SELECT 1"]],
    ["empty statements", ";; SELECT 1;;", ["SELECT 1"]],
    [
      "dollar quoted",
      "CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql; SELECT f()",
      ["CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql", "SELECT f()"],
    ],
    ["tagged dollar", "SELECT $body$ a;b $body$; SELECT 2", ["SELECT $body$ a;b $body$", "SELECT 2"]],
    ["positional param", "SELECT $1; SELECT 2", ["SELECT $1", "SELECT 2"]],
    [
      "mysql delimiter",
      "DELIMITER //\nCREATE PROCEDURE p() BEGIN SELECT 1; SELECT 2; END//\nDELIMITER ;\nCALL p();",
      ["CREATE PROCEDURE p() BEGIN SELECT 1; SELECT 2; END", "CALL p()"],
    ],
  ];
  it.each(cases)("%s", (_name, input, want) => {
    expect(splitStatements(input)).toEqual(want);
  });
});

describe("isReadOnly", () => {
  it.each([
    ["SELECT 1", true],
    ["  (SELECT 1)", true],
    ["-- c\nSELECT 1", true],
    ["WITH x AS (SELECT 1) SELECT * FROM x", true],
    ["SHOW TABLES", true],
    ["EXPLAIN SELECT 1", true],
    ["EXPLAIN ANALYZE DELETE FROM t", false],
    ["EXPLAIN SELECT analyzer FROM t", true],
    ["ANALYZE TABLE t", false],
    ["/* SELECT */ DELETE FROM t", false],
    ["SELECTED FROM t", false],
    ["INSERT INTO t VALUES (1)", false],
    ["", false],
  ])("%j -> %s", (sql, want) => {
    expect(isReadOnly(sql as string)).toBe(want);
  });
});

describe("writeStatements", () => {
  it("returns only the writes of a mixed script", () => {
    expect(writeStatements("SELECT 1; UPDATE t SET a = 1; SELECT 'x;y'; DROP TABLE u")).toEqual([
      "UPDATE t SET a = 1",
      "DROP TABLE u",
    ]);
  });
  it("is empty for a read-only script", () => {
    expect(writeStatements("SELECT 1; SHOW TABLES")).toEqual([]);
  });
});

describe("leadingKeyword", () => {
  it("skips nested comments and parentheses", () => {
    expect(leadingKeyword("/* a */ -- b\n # c\n ((select 1))")).toBe("SELECT");
  });
});
