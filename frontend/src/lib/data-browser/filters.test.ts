import { describe, expect, it } from "vitest";

import { describeValue, fromDraft, MAX_LIST, parseList, toDraft } from "./filters";

describe("filters", () => {
  it("parses typed lists", () => {
    expect(parseList("a, b\nc,, ")).toEqual(["a", "b", "c"]);
    expect(parseList(Array.from({ length: MAX_LIST + 5 }, (_, i) => i).join(","))).toHaveLength(MAX_LIST);
  });

  it("round-trips lists and ranges through the editor", () => {
    const list = { column: "id", op: "in" as const, values: ["1", "2"] };
    expect(toDraft(list)).toEqual({ column: "id", op: "in", value: "1, 2" });
    expect(fromDraft(toDraft(list))).toEqual(list);
    const range = { column: "id", op: "between" as const, values: ["1", "9"] };
    expect(fromDraft(toDraft(range))).toEqual(range);
  });

  it("drops incomplete filters", () => {
    expect(fromDraft({ column: "id", op: "in", value: " , " })).toBeNull();
    expect(fromDraft({ column: "id", op: "between", values: ["1", ""] })).toBeNull();
    expect(fromDraft({ column: "id", op: "eq", value: "" })).toBeNull();
    expect(fromDraft({ column: "", op: "is_null" })).toBeNull();
    expect(fromDraft({ column: "id", op: "is_null", value: "x" })).toEqual({ column: "id", op: "is_null" });
  });

  it("describes operands for chips", () => {
    expect(describeValue({ column: "a", op: "in", values: ["1", "2", "3", "4", "5"] })).toBe("1, 2, 3 +2");
    expect(describeValue({ column: "a", op: "between", values: ["1", "9"] })).toBe("1 … 9");
    expect(describeValue({ column: "a", op: "not_null" })).toBe("");
  });
});
