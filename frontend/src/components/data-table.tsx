"use client";

// Shared table for list pages. Not used for: the Data Browser grid (dynamic columns), expandable
// DB user rows, and roles card layout.
import { ArrowDown, ArrowUp, RefreshCw, Search, WifiOff } from "lucide-react";
import type { ReactNode } from "react";

import { Button, cx, EmptyState, Pagination, SearchInput, Table, TableSkeleton, Toolbar } from "@/components/ui";
import type { SortDir } from "@/lib/use-data-table";

export type DataTableColumn<T> = {
  id: string;
  header: string;
  sortable?: boolean;
  sortKey?: string;
  align?: "left" | "right";
  className?: string;
  /** Hide this column on phone-sized screens (secondary details). */
  hideOnMobile?: boolean;
  render: (row: T) => ReactNode;
};

export type DataTableProps<T> = {
  columns: DataTableColumn<T>[];
  rows: T[];
  rowKey: (row: T) => string;
  isLoading?: boolean;
  loadingLabel?: string;
  error?: string;
  errorTitle?: string;
  emptyTitle: string;
  emptyHint?: string;
  emptySearchTitle?: string;
  emptySearchHint?: string;
  /** The next step when there is nothing to show (e.g. a "Connect" button). */
  emptyAction?: ReactNode;
  search?: {
    value: string;
    onChange: (value: string) => void;
    placeholder?: string;
  };
  sort?: {
    key: string;
    dir: SortDir;
    onSort: (key: string) => void;
  };
  pagination?: {
    page: number;
    pageCount: number;
    onPage: (page: number) => void;
    hasMore?: boolean;
  };
  toolbar?: ReactNode;
  /** Shows a "Try again" button on the error state. */
  onRetry?: () => void;
};

function SortableHeader({
  label,
  sortKey,
  activeKey,
  dir,
  onSort,
  className,
}: {
  label: string;
  sortKey: string;
  activeKey: string;
  dir: SortDir;
  onSort: (key: string) => void;
  className?: string;
}) {
  const active = sortKey === activeKey;
  return (
    <th className={className} aria-sort={active ? (dir === "asc" ? "ascending" : "descending") : "none"}>
      <button type="button" className="th-sort" onClick={() => onSort(sortKey)}>
        {label}
        {active ? dir === "asc" ? <ArrowUp size={12} aria-hidden /> : <ArrowDown size={12} aria-hidden /> : null}
      </button>
    </th>
  );
}

export function DataTable<T>({
  columns,
  rows,
  rowKey,
  isLoading,
  loadingLabel = "Loading…",
  error,
  errorTitle = "Could not load data",
  emptyTitle,
  emptyHint,
  emptySearchTitle,
  emptySearchHint,
  emptyAction,
  search,
  sort,
  pagination,
  toolbar,
  onRetry,
}: DataTableProps<T>) {
  const searchActive = Boolean(search?.value.trim());

  if (isLoading) {
    return (
      <div role="status" aria-label={loadingLabel}>
        <TableSkeleton columns={Math.min(columns.length, 5)} />
      </div>
    );
  }

  if (error) {
    return (
      <EmptyState
        icon={<WifiOff size={22} />}
        title={errorTitle}
        hint={error}
        action={
          onRetry ? (
            <Button variant="primary" icon={<RefreshCw size={14} aria-hidden />} onClick={onRetry}>
              Try again
            </Button>
          ) : undefined
        }
      />
    );
  }

  return (
    <div>
      {search || toolbar ? (
        <Toolbar>
          {search ? (
            <SearchInput
              icon={<Search size={15} aria-hidden />}
              placeholder={search.placeholder ?? "Search…"}
              aria-label={search.placeholder ?? "Search"}
              value={search.value}
              onChange={(e) => search.onChange(e.target.value)}
            />
          ) : null}
          {toolbar}
        </Toolbar>
      ) : null}

      {rows.length === 0 ? (
        <EmptyState
          title={searchActive && emptySearchTitle ? emptySearchTitle : emptyTitle}
          hint={searchActive && emptySearchTitle ? emptySearchHint : emptyHint}
          action={searchActive ? undefined : emptyAction}
        />
      ) : (
        <>
          <Table>
              <thead>
                <tr>
                  {columns.map((col) =>
                    col.sortable && sort && col.sortKey ? (
                      <SortableHeader
                        key={col.id}
                        label={col.header}
                        sortKey={col.sortKey}
                        activeKey={sort.key}
                        dir={sort.dir}
                        onSort={sort.onSort}
                        className={cx(col.hideOnMobile && "hide-sm", col.align === "right" && "text-right") || undefined}
                      />
                    ) : (
                      <th
                        key={col.id}
                        className={cx(col.hideOnMobile && "hide-sm", col.align === "right" && "text-right") || undefined}
                      >
                        {col.header}
                      </th>
                    ),
                  )}
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={rowKey(row)}>
                    {columns.map((col) => (
                      <td
                        key={col.id}
                        className={cx(col.className, col.hideOnMobile && "hide-sm", col.align === "right" && "text-right") || undefined}
                      >
                        {col.render(row)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
          </Table>
          {pagination ? (
            <div className="flex items-center justify-end mt-2.5">
              <Pagination
                page={pagination.page}
                pageCount={pagination.pageCount}
                hasMore={pagination.hasMore}
                onPage={pagination.onPage}
              />
            </div>
          ) : null}
        </>
      )}
    </div>
  );
}
