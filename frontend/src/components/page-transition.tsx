"use client";

import { ViewTransition, useLayoutEffect, type ReactNode } from "react";

// Route-loading skeletons on screen; read during render to pick the page's enter animation.
let skeletonsShown = 0;

/**
 * PageTransition fades pages out and in on navigation (styles in base.css). It is the
 * default export of the dashboard's template.tsx files, which remount on page changes only.
 */
export function PageTransition({ children }: { children: ReactNode }) {
  const enter = skeletonsShown > 0 ? "page-reveal" : "page-enter";
  return (
    <ViewTransition enter={enter} exit="page-exit" default="none">
      {children}
    </ViewTransition>
  );
}

/** RouteLoading wraps a loading.tsx skeleton so the page replacing it only fades in. */
export function RouteLoading({ children }: { children: ReactNode }) {
  useLayoutEffect(() => {
    skeletonsShown += 1;
    return () => {
      skeletonsShown -= 1;
    };
  }, []);
  return (
    <ViewTransition exit="skeleton-exit" default="none">
      {children}
    </ViewTransition>
  );
}
