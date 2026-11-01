import type { ReactNode } from "react";

/** Skeleton is a placeholder block shown while content loads. */
export function Skeleton({
  width = "100%",
  height = "1rem",
  className,
  radius,
}: {
  width?: string | number;
  height?: string | number;
  className?: string;
  radius?: string | number;
}) {
  return (
    <span
      className={`skeleton${className ? ` ${className}` : ""}`}
      style={{ width, height, borderRadius: radius }}
      aria-hidden
    />
  );
}

/** TableSkeleton mimics a table while its rows load. */
export function TableSkeleton({
  rows = 5,
  columns = 4,
  bare,
}: {
  rows?: number;
  columns?: number;
  bare?: boolean;
}) {
  return (
    <div
      className={`${bare ? "" : "card "}table-wrap`}
      role="status"
      aria-label="Loading"
    >
      <table className="table">
        <thead>
          <tr>
            {Array.from({ length: columns }, (_, c) => (
              <th key={c}>
                <Skeleton width={c === 0 ? "5rem" : "3.5rem"} height=".7rem" />
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {Array.from({ length: rows }, (_, r) => (
            <tr key={r}>
              {Array.from({ length: columns }, (_, c) => (
                <td key={c}>
                  <Skeleton
                    width={
                      c === 0
                        ? `${55 + ((r * 13) % 30)}%`
                        : `${30 + ((r * 7 + c * 11) % 30)}%`
                    }
                  />
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** StatStripSkeleton mimics a row of StatTiles. */
export function StatStripSkeleton({ count = 4 }: { count?: number }) {
  return (
    <div className="stat-strip" aria-hidden>
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="card stat-tile">
          <Skeleton width="45%" height=".7rem" />
          <Skeleton width="35%" height="1.5rem" />
          <Skeleton width="60%" height=".7rem" />
        </div>
      ))}
    </div>
  );
}

/** HeaderSkeleton mimics a PageHeader. */
export function HeaderSkeleton({
  breadcrumbs,
  actions = true,
}: {
  breadcrumbs?: boolean;
  actions?: boolean;
}) {
  return (
    <div className="page-header" aria-hidden>
      {breadcrumbs ? (
        <Skeleton width="10rem" height=".75rem" className="mb-3" />
      ) : null}
      <div className="page-header-row">
        <div className="page-header-main stack-sm">
          <Skeleton width="14rem" height="1.4rem" />
          <Skeleton width="22rem" height=".85rem" />
        </div>
        {actions ? (
          <Skeleton width="8rem" height="2.25rem" radius="var(--radius-md)" />
        ) : null}
      </div>
    </div>
  );
}

/** PageSkeleton is the loading state for a list page: header, filters, table. */
export function PageSkeleton({
  label = "Loading",
  stats,
  children,
}: {
  label?: string;
  stats?: boolean;
  children?: ReactNode;
}) {
  return (
    <div role="status" aria-label={label}>
      <HeaderSkeleton />
      {stats ? <StatStripSkeleton /> : null}
      {children ?? (
        <>
          <div className="toolbar" aria-hidden>
            <Skeleton
              width="18rem"
              height="2.25rem"
              radius="var(--radius-md)"
            />
          </div>
          <TableSkeleton rows={6} columns={5} />
        </>
      )}
    </div>
  );
}

/** DetailSkeleton is the loading state for a detail page: breadcrumbs, stats, main + aside. */
export function DetailSkeleton({ label = "Loading" }: { label?: string }) {
  return (
    <div role="status" aria-label={label}>
      <HeaderSkeleton breadcrumbs />
      <StatStripSkeleton />
      <div className="detail-grid" aria-hidden>
        <div className="stack">
          <div className="card panel stack-sm">
            <Skeleton width="30%" height=".9rem" />
            <Skeleton width="80%" />
            <Skeleton width="65%" />
            <Skeleton width="72%" />
          </div>
          <TableSkeleton rows={4} columns={4} />
        </div>
        <div className="card panel stack-sm">
          <Skeleton width="50%" height=".9rem" />
          <Skeleton width="90%" />
          <Skeleton width="70%" />
        </div>
      </div>
    </div>
  );
}
