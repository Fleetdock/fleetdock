"use client";

// Shared table for list pages. Deferred: DataBrowser (dynamic columns), expandable
// DB user rows, and roles card layout.
import type { ReactNode } from "react";

import { ArrowDown, ArrowUp, Search } from "lucide-react";

import { EmptyState, Pagination, TableSkeleton } from "@/components/ui";
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
      <button
        type="button"
        className="btn btn-ghost btn-sm"
        style={{
          padding: 0,
          font: "inherit",
          color: "inherit",
          textTransform: "inherit",
          letterSpacing: "inherit",
          fontWeight: 500,
          display: "inline-flex",
          alignItems: "center",
          gap: ".25rem",
        }}
        onClick={() => onSort(sortKey)}
      >
        {label}
        {active ? (dir === "asc" ? <ArrowUp size={12} /> : <ArrowDown size={12} />) : null}
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
}: DataTableProps<T>) {
  const searchActive = Boolean(search?.value.trim());

  if (isLoading) {
    return (
      <div aria-label={loadingLabel}>
        <TableSkeleton columns={Math.min(columns.length, 5)} />
      </div>
    );
  }

  if (error) {
    return <EmptyState title={errorTitle} hint={error} />;
  }

  return (
    <div>
      {search || toolbar ? (
        <div className="flex items-center gap-2 flex-wrap" style={{ marginBottom: ".9rem" }}>
          {search ? (
            <div style={{ position: "relative", maxWidth: "22rem", width: "100%" }}>
              <span style={{ position: "absolute", left: 10, top: 9, color: "var(--muted)" }}>
                <Search size={16} />
              </span>
              <input
                className="input"
                style={{ paddingLeft: "2rem", width: "100%" }}
                placeholder={search.placeholder ?? "Search…"}
                aria-label={search.placeholder ?? "Search"}
                type="search"
                value={search.value}
                onChange={(e) => search.onChange(e.target.value)}
              />
            </div>
          ) : null}
          {toolbar}
        </div>
      ) : null}

      {rows.length === 0 ? (
        <EmptyState
          title={searchActive && emptySearchTitle ? emptySearchTitle : emptyTitle}
          hint={searchActive && emptySearchTitle ? emptySearchHint : emptyHint}
          action={searchActive ? undefined : emptyAction}
        />
      ) : (
        <>
          <div className="card table-wrap">
            <table className="table">
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
                        className={col.hideOnMobile ? "hide-sm" : undefined}
                      />
                    ) : (
                      <th
                        key={col.id}
                        className={col.hideOnMobile ? "hide-sm" : undefined}
                        style={col.align === "right" ? { textAlign: "right" } : undefined}
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
                        className={[col.className, col.hideOnMobile ? "hide-sm" : ""].filter(Boolean).join(" ") || undefined}
                        style={col.align === "right" ? { textAlign: "right" } : undefined}
                      >
                        {col.render(row)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {pagination ? (
            <div className="flex items-center justify-end" style={{ marginTop: ".6rem" }}>
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
