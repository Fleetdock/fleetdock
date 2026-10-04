"use client";

import Link from "next/link";
import type { ReactNode } from "react";
import { ChevronRight } from "lucide-react";

export type Crumb = { label: string; href?: string };

/** Breadcrumbs shows where a page sits; the last crumb is the current page. */
export function Breadcrumbs({ items }: { items: Crumb[] }) {
  if (items.length === 0) return null;
  return (
    <nav aria-label="Breadcrumb" className="breadcrumbs">
      <ol>
        {items.map((c, i) => {
          const last = i === items.length - 1;
          return (
            <li key={`${c.label}-${i}`}>
              {c.href && !last ? <Link href={c.href}>{c.label}</Link> : <span aria-current={last ? "page" : undefined}>{c.label}</span>}
              {!last ? <ChevronRight size={13} aria-hidden /> : null}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}

/**
 * PageHeader is the top of every page: where you are, what this page is for,
 * and its main actions. Actions wrap under the title on narrow screens.
 */
export function PageHeader({
  title,
  description,
  actions,
  breadcrumbs,
  badges,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  breadcrumbs?: Crumb[];
  badges?: ReactNode;
}) {
  return (
    <header className="page-header">
      {breadcrumbs ? <Breadcrumbs items={breadcrumbs} /> : null}
      <div className="page-header-row">
        <div style={{ minWidth: 0 }}>
          <div className="flex items-center gap-2" style={{ flexWrap: "wrap" }}>
            <h1 className="text-xl font-semibold">{title}</h1>
            {badges}
          </div>
          {description ? <p className="muted text-sm" style={{ marginTop: ".2rem" }}>{description}</p> : null}
        </div>
        {actions ? <div className="page-header-actions">{actions}</div> : null}
      </div>
    </header>
  );
}
