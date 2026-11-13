import { describe, expect, it } from "vitest";

import { nestPartitions, partitionCount } from "./partitions";

const items = [
  { name: "public.events" },
  { name: "public.events_2026_01", partitionOf: "public.events" },
  { name: "public.events_2026_02", partitionOf: "public.events" },
  { name: "public.events_2026_02_a", partitionOf: "public.events_2026_02" },
  { name: "public.users" },
  { name: "public.orphan", partitionOf: "other.parent" },
];
const names = (l: { item: { name: string }; depth: number }[]) => l.map((x) => `${x.depth}:${x.item.name}`);

describe("nestPartitions", () => {
  it("hides partitions of closed tables", () => {
    expect(names(nestPartitions(items, new Set(), false))).toEqual(["0:public.events", "0:public.users", "0:public.orphan"]);
  });
  it("lists partitions under an open table, nested", () => {
    expect(names(nestPartitions(items, new Set(["public.events", "public.events_2026_02"]), false))).toEqual([
      "0:public.events",
      "1:public.events_2026_01",
      "1:public.events_2026_02",
      "2:public.events_2026_02_a",
      "0:public.users",
      "0:public.orphan",
    ]);
  });
  it("lists search matches flat", () => {
    expect(nestPartitions(items, new Set(), true)).toHaveLength(items.length);
  });
  it("counts direct partitions", () => {
    expect(partitionCount(items).get("public.events")).toBe(2);
  });
});
