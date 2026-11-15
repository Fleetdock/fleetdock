import { describe, expect, it } from "vitest";

import { parseSearchPath } from "./search-path";

describe("parseSearchPath", () => {
  it("splits PostgreSQL's printed form", () => {
    expect(parseSearchPath('"$user", public')).toEqual(["$user", "public"]);
    expect(parseSearchPath("sales, public")).toEqual(["sales", "public"]);
    expect(parseSearchPath('"we""ird", "Mixed Case"')).toEqual(['we"ird', "Mixed Case"]);
    expect(parseSearchPath("")).toEqual([]);
  });
});
