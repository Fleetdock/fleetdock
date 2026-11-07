import { describe, expect, it } from "vitest";

import { loginURL, safeNext } from "./api";

describe("safeNext", () => {
  it("keeps paths on this site", () => {
    expect(safeNext("/data?db=1&table=orders")).toBe("/data?db=1&table=orders");
    expect(safeNext("/databases/abc#schema")).toBe("/databases/abc#schema");
  });
  it("refuses anything that could leave the site or loop", () => {
    for (const bad of [
      null,
      "",
      "https://evil.example/",
      "//evil.example/x",
      "/\\evil.example",
      "\\\\evil.example",
      "javascript:alert(1)",
      "data",
      "/login",
      "/login?next=/x",
    ]) {
      expect(safeNext(bad)).toBeNull();
    }
  });
});

describe("loginURL", () => {
  it("says the session ended and where to return", () => {
    expect(loginURL("/data?db=1")).toBe("/login?expired=1&next=%2Fdata%3Fdb%3D1");
    expect(loginURL("/dashboard")).toBe("/login?expired=1");
    expect(loginURL("//evil.example")).toBe("/login?expired=1");
  });
});
