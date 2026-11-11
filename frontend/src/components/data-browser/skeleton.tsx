"use client";

import { useLayoutEffect, useState } from "react";

import { Skeleton } from "@/components/ui";
import { load } from "@/lib/data-browser/storage";

// Data Browser skeletons, built from its real layout classes so sizes match.

const ROW_WIDTHS = [62, 48, 75, 55, 68, 42, 80, 58, 50, 70, 45, 64];

/** TreeRowsSkeleton stands in for the rows of the table list while it loads. */
export function TreeRowsSkeleton({ rows = 12 }: { rows?: number }) {
  return (
    <div role="status" aria-label="Loading tables">
      {ROW_WIDTHS.slice(0, rows).map((w, i) => (
        <div key={i} className="dbx-tree-item dbx-skeleton-row" aria-hidden>
          <Skeleton width="16px" height="16px" radius="4px" />
          <Skeleton width={`${w}%`} height=".7rem" />
        </div>
      ))}
    </div>
  );
}

/** TreeSkeleton is the sidebar below the database picker: head, search, rows. */
export function TreeSkeleton() {
  return (
    <div className="dbx-tree" aria-hidden>
      <div className="dbx-tree-head dbx-skeleton-head">
        <Skeleton width="5.5rem" height=".65rem" />
        <span className="flex items-center gap-1">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} width="1.1rem" height="1.1rem" radius="var(--radius-sm)" />
          ))}
        </span>
      </div>
      <div className="mx-[.6rem] mb-[.4rem]">
        <Skeleton height="2rem" radius="var(--radius-md)" />
      </div>
      <div className="dbx-tree-list">
        <TreeRowsSkeleton />
      </div>
    </div>
  );
}

/** GridSkeleton stands in for the grid while the first page of rows loads. */
export function GridSkeleton() {
  return (
    <div className="dbx-grid-skeleton" role="status" aria-label="Loading rows">
      <div className="dbx-grid-skeleton-head" aria-hidden>
        {[7, 10, 6, 9, 8].map((w, i) => (
          <Skeleton key={i} width={`${w}rem`} height=".8rem" />
        ))}
      </div>
      {Array.from({ length: 12 }, (_, r) => (
        <div key={r} className="dbx-grid-skeleton-row" aria-hidden>
          {[5, 9, 4, 7, 6].map((w, i) => (
            <Skeleton key={i} width={`${w + ((r * 3 + i) % 4)}rem`} height=".7rem" />
          ))}
        </div>
      ))}
    </div>
  );
}

/** MainSkeleton is the main area: tab bar, table toolbar and grid. */
export function MainSkeleton() {
  return (
    <div className="dbx-skeleton-main" role="status" aria-label="Loading Data Browser">
      <div className="dbx-tabbar" aria-hidden>
        <div className="dbx-tabs">
          {["5.5rem", "4rem"].map((w, i) => (
            <div key={i} className="dbx-tab dbx-skeleton-row">
              <Skeleton width="13px" height="13px" radius="3px" />
              <Skeleton width={w} height=".65rem" />
            </div>
          ))}
        </div>
      </div>
      <div className="dbx-panes" aria-hidden>
        <div className="dbx-pane">
          <div className="dbx-toolbar">
            <Skeleton width="9.5rem" height="2.15rem" radius="var(--radius-md)" />
            <Skeleton width="16rem" height="2rem" radius="var(--radius-md)" />
          </div>
          <div className="dbx-body">
            <div className="dbx-grid-wrap">
              <GridSkeleton />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

/** DataBrowserSkeleton is the whole workspace, with the sidebar width the user last chose. */
export function DataBrowserSkeleton() {
  const [side, setSide] = useState<{ width: number; collapsed: boolean }>({ width: 264, collapsed: false });
  useLayoutEffect(() => {
    const w = Number(load("sidebar-width"));
    // Same bounds as useSidebar in workspace.tsx.
    // Storage isn't readable during server rendering, so read it here.
    setSide({ width: w >= 200 && w <= 520 ? w : 264, collapsed: load("sidebar-collapsed") === "1" });
  }, []);
  return (
    <div className={`dbx${side.collapsed ? " side-collapsed" : ""}`}>
      <aside className="dbx-side" style={{ width: side.width }} aria-hidden>
        <div className="dbx-side-head">
          <div className="dbx-picker-button dbx-skeleton-row">
            <Skeleton width="1.6rem" height="1.6rem" radius="var(--radius-sm)" />
            <span className="flex flex-col gap-1.5 flex-1">
              <Skeleton width="70%" height=".65rem" />
              <Skeleton width="45%" height=".55rem" />
            </span>
          </div>
        </div>
        <TreeSkeleton />
      </aside>
      <section className="dbx-main">
        <MainSkeleton />
      </section>
    </div>
  );
}
