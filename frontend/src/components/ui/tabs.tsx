"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import type { ReactNode } from "react";

export type TabItem = {
  label: ReactNode;
  /** Route tab: matched against the current path. */
  href: string;
  /** Match only the exact path (for a section's index tab). */
  exact?: boolean;
  hidden?: boolean;
};

/**
 * TabNav renders route-based tabs: each tab is a link, so tabs deep-link,
 * work with the back button and can be opened in a new tab.
 */
export function TabNav({ items, label }: { items: TabItem[]; label: string }) {
  const pathname = usePathname();
  const visible = items.filter((t) => !t.hidden);
  if (visible.length <= 1) return null;
  return (
    <nav className="tabs" aria-label={label}>
      {visible.map((t) => {
        const active = t.exact ? pathname === t.href : pathname === t.href || pathname.startsWith(`${t.href}/`);
        return (
          <Link key={t.href} href={t.href} className={`tab${active ? " active" : ""}`} aria-current={active ? "page" : undefined}>
            {t.label}
          </Link>
        );
      })}
    </nav>
  );
}

export type QueryTab = { id: string; label: ReactNode; hidden?: boolean };

/**
 * QueryTabs are tabs within one page, kept in a URL query parameter so the
 * selected tab survives reloads and the back button.
 */
export function QueryTabs({
  tabs,
  param = "tab",
  label,
  onSelect,
}: {
  tabs: QueryTab[];
  param?: string;
  label: string;
  onSelect: (id: string) => void;
}) {
  const sp = useSearchParams();
  const visible = tabs.filter((t) => !t.hidden);
  const current = sp.get(param) ?? visible[0]?.id;
  return (
    <div className="tabs" role="tablist" aria-label={label}>
      {visible.map((t) => (
        <button
          key={t.id}
          type="button"
          role="tab"
          aria-selected={current === t.id}
          className={`tab${current === t.id ? " active" : ""}`}
          onClick={() => onSelect(t.id)}
        >
          {t.label}
        </button>
      ))}
    </div>
  );
}
