"use client";

import { useMemo, useState } from "react";

import { LIST_PAGE_SIZE } from "./hooks";

export type SortDir = "asc" | "desc";

export function useDataTable<T>(options: {
  items: T[] | undefined;
  pageSize?: number;
  search?: { query: string; match: (row: T, q: string) => boolean };
  sort?: {
    key: string;
    dir: SortDir;
    compare: (a: T, b: T, key: string) => number;
    defaultDir?: (key: string) => SortDir;
  };
}) {
  const pageSize = options.pageSize ?? LIST_PAGE_SIZE;
  const [page, setPage] = useState(1);
  const [sortKey, setSortKey] = useState(options.sort?.key ?? "");
  const [sortDir, setSortDir] = useState<SortDir>(options.sort?.dir ?? "asc");

  const filtered = useMemo(() => {
    const all = options.items ?? [];
    const q = options.search?.query.trim().toLowerCase() ?? "";
    if (!q || !options.search) return all;
    return all.filter((row) => options.search!.match(row, q));
  }, [options.items, options.search]);

  const sorted = useMemo(() => {
    if (!options.sort || !sortKey) return filtered;
    const items = [...filtered];
    items.sort((a, b) => {
      const cmp = options.sort!.compare(a, b, sortKey);
      return sortDir === "asc" ? cmp : -cmp;
    });
    return items;
  }, [filtered, options.sort, sortKey, sortDir]);

  const pageCount = Math.max(1, Math.ceil(sorted.length / pageSize));
  const current = Math.min(page, pageCount);
  const rows = sorted.slice((current - 1) * pageSize, current * pageSize);

  function setSort(key: string) {
    if (key === sortKey) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir(options.sort?.defaultDir?.(key) ?? "asc");
    }
    setPage(1);
  }

  return {
    rows,
    page: current,
    setPage,
    pageCount,
    total: sorted.length,
    filteredTotal: filtered.length,
    sortKey,
    sortDir,
    setSort,
  };
}
