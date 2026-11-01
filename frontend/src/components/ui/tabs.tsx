"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { useRef, type KeyboardEvent, type ReactNode } from "react";

export type TabItem = {
  label: ReactNode;
  /** Route tab: matched against the current path. */
  href: string;
  /** Match only the exact path (for a section's index tab). */
  exact?: boolean;
  hidden?: boolean;
};

// moveFocus implements arrow/Home/End navigation within a row of tabs.
function moveFocus(
  e: KeyboardEvent<HTMLElement>,
  selector: string,
  activate?: (el: HTMLElement) => void,
) {
  const keys = ["ArrowLeft", "ArrowRight", "Home", "End"];
  if (!keys.includes(e.key)) return;
  const items = Array.from(
    e.currentTarget.querySelectorAll<HTMLElement>(selector),
  );
  const i = items.indexOf(document.activeElement as HTMLElement);
  if (i < 0) return;
  e.preventDefault();
  const next =
    e.key === "Home"
      ? 0
      : e.key === "End"
        ? items.length - 1
        : (i + (e.key === "ArrowRight" ? 1 : -1) + items.length) % items.length;
  items[next].focus();
  activate?.(items[next]);
}

/**
 * TabNav renders route-based tabs: each tab is a link, so tabs deep-link,
 * work with the back button and can be opened in a new tab. Arrow keys move
 * between them.
 */
export function TabNav({ items, label }: { items: TabItem[]; label: string }) {
  const pathname = usePathname();
  const visible = items.filter((t) => !t.hidden);
  if (visible.length <= 1) return null;
  return (
    <nav
      className="tabs"
      aria-label={label}
      onKeyDown={(e) => moveFocus(e, "a.tab")}
    >
      {visible.map((t) => {
        const active = t.exact
          ? pathname === t.href
          : pathname === t.href || pathname.startsWith(`${t.href}/`);
        return (
          <Link
            key={t.href}
            href={t.href}
            className={`tab${active ? " active" : ""}`}
            aria-current={active ? "page" : undefined}
          >
            {t.label}
          </Link>
        );
      })}
    </nav>
  );
}

export type TabOption<T extends string = string> = {
  id: T;
  label: ReactNode;
  hidden?: boolean;
  count?: number;
};

/**
 * Tabs is a WAI-ARIA tablist for switching views inside a page. Only the
 * selected tab is in the Tab order; arrow keys, Home and End move between
 * tabs and select them. Pass `panelId` (the id of the element holding the
 * content) so each tab is linked to it.
 */
export function Tabs<T extends string>({
  tabs,
  value,
  onChange,
  label,
  panelId,
  className,
}: {
  tabs: TabOption<T>[];
  value: T | undefined;
  onChange: (id: T) => void;
  label: string;
  panelId?: string;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const visible = tabs.filter((t) => !t.hidden);
  const current = visible.some((t) => t.id === value) ? value : visible[0]?.id;
  return (
    <div
      ref={ref}
      className={`tabs${className ? ` ${className}` : ""}`}
      role="tablist"
      aria-label={label}
      onKeyDown={(e) =>
        moveFocus(e, '[role="tab"]', (el) => onChange(el.dataset.tab as T))
      }
    >
      {visible.map((t) => {
        const selected = current === t.id;
        return (
          <button
            key={t.id}
            type="button"
            role="tab"
            data-tab={t.id}
            aria-selected={selected}
            aria-controls={panelId}
            tabIndex={selected ? 0 : -1}
            className={`tab${selected ? " active" : ""}`}
            onClick={() => onChange(t.id)}
          >
            {t.label}
            {t.count !== undefined ? (
              <span className="tab-count">{t.count}</span>
            ) : null}
          </button>
        );
      })}
    </div>
  );
}

export type QueryTab = TabOption;

/**
 * QueryTabs are tabs within one page, kept in a URL query parameter so the
 * selected tab survives reloads and the back button.
 */
export function QueryTabs({
  tabs,
  param = "tab",
  label,
  onSelect,
  panelId,
}: {
  tabs: QueryTab[];
  param?: string;
  label: string;
  onSelect: (id: string) => void;
  panelId?: string;
}) {
  const sp = useSearchParams();
  return (
    <Tabs
      tabs={tabs}
      value={sp.get(param) ?? undefined}
      onChange={onSelect}
      label={label}
      panelId={panelId}
    />
  );
}
