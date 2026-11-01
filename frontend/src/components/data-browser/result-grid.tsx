"use client";

import { useCallback, useMemo, useState } from "react";

import type { Dialect } from "@/lib/data-browser/cells";
import { resultColumns, sortRowIndexes, withNumericTypes } from "@/lib/data-browser/results";
import type { QueryResult, SortKey } from "@/lib/types";

import { DataGrid, type Cell, type GridRow } from "./data-grid";

const noop = () => undefined;

/**
 * ResultGrid shows a console statement's rows in the data browser's grid:
 * keyboard navigation, copy as TSV/JSON/INSERT, resizable columns and sorting
 * (done here, on the rows already fetched). Results are read-only.
 */
export function ResultGrid({ result, dialect }: { result: QueryResult; dialect: Dialect }) {
  const columns = useMemo(() => withNumericTypes(resultColumns(result.columns), result.rows), [result.columns, result.rows]);
  const [sort, setSort] = useState<SortKey[]>([]);
  const [widths, setWidths] = useState<Record<string, number>>({});
  const [cursor, setCursor] = useState<Cell | null>(null);
  const [selected, setSelected] = useState<Set<number>>(new Set());

  const rows = useMemo<GridRow[]>(() => {
    const order = sortRowIndexes(result.rows, columns, sort);
    return order.map((i, pos) => ({ kind: "data", index: pos, cells: result.rows[i], rowId: null, deleted: false }));
  }, [result.rows, columns, sort]);

  const onSort = useCallback((column: string, dir: "asc" | "desc" | "toggle" | "clear", additive: boolean) => {
    setSort((cur) => {
      const has = cur.find((s) => s.column === column);
      if (dir === "clear") return cur.filter((s) => s.column !== column);
      if (dir === "asc" || dir === "desc") return [{ column, desc: dir === "desc" }];
      const next: SortKey | null = !has ? { column, desc: false } : !has.desc ? { column, desc: true } : null;
      if (!additive) return next ? [next] : [];
      return has ? cur.flatMap((s) => (s.column === column ? (next ? [next] : []) : [s])) : [...cur, next!];
    });
    setSelected(new Set());
  }, []);

  return (
    <DataGrid
      columns={columns}
      rows={rows}
      keyCols={[]}
      offset={0}
      dialect={dialect}
      quotedTable="query_result"
      sort={sort}
      onSort={onSort}
      widths={widths}
      onWidths={setWidths}
      fks={{}}
      editable={false}
      cursor={cursor}
      setCursor={setCursor}
      selected={selected}
      setSelected={setSelected}
      onEdit={noop}
      onRevertCell={noop}
      onDeleteRows={noop}
      onRestoreRows={noop}
      onDuplicate={noop}
      onFollow={noop}
      incoming={[]}
      onReferencing={noop}
      emptyText="No rows returned."
      editRequest={null}
      onEditRequestDone={noop}
    />
  );
}
