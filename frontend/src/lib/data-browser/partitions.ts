// Partitions in the data browser's table tree (PostgreSQL): a partition is
// listed under its partitioned table, which opens and closes like a folder.

export type Partitionable = {
  name: string;
  /** API name of the partitioned table this item is a partition of. */
  partitionOf?: string;
};

/**
 * nestPartitions puts each partition right after its parent, only when the
 * parent is open, and gives every item its depth. While searching, matches
 * are listed flat (depth 0), as they are found. A partition whose parent is
 * not in the list (another schema, or filtered out) stays at the top level.
 */
export function nestPartitions<T extends Partitionable>(
  items: T[],
  open: ReadonlySet<string>,
  searching: boolean,
): { item: T; depth: number }[] {
  if (searching) return items.map((item) => ({ item, depth: 0 }));
  const names = new Set(items.map((i) => i.name));
  const children = new Map<string, T[]>();
  for (const i of items) {
    if (i.partitionOf && names.has(i.partitionOf)) {
      const list = children.get(i.partitionOf) ?? [];
      list.push(i);
      children.set(i.partitionOf, list);
    }
  }
  const out: { item: T; depth: number }[] = [];
  const add = (i: T, depth: number) => {
    out.push({ item: i, depth });
    if (open.has(i.name)) for (const c of children.get(i.name) ?? []) add(c, depth + 1);
  };
  for (const i of items) {
    if (i.partitionOf && names.has(i.partitionOf)) continue;
    add(i, 0);
  }
  return out;
}

/** partitionCount counts each partitioned table's direct partitions. */
export function partitionCount(items: Partitionable[]): Map<string, number> {
  const n = new Map<string, number>();
  for (const i of items) if (i.partitionOf) n.set(i.partitionOf, (n.get(i.partitionOf) ?? 0) + 1);
  return n;
}
