"use client";

import Link from "next/link";
import type { ReactNode } from "react";

import { CopyButton } from "./base";

/**
 * SectionHeader is the heading row of a page section: a title with an
 * optional icon and description on the left, actions on the right. Use it so
 * every section on a detail page reads the same way.
 */
export function SectionHeader({
  title,
  icon,
  description,
  actions,
  as: Heading = "h2",
}: {
  title: ReactNode;
  icon?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  as?: "h2" | "h3";
}) {
  return (
    <div className="section-header">
      <div style={{ minWidth: 0 }}>
        <Heading className="section-header-title">
          {icon ? <span className="section-header-icon">{icon}</span> : null}
          {title}
        </Heading>
        {description ? <p className="section-header-description">{description}</p> : null}
      </div>
      {actions ? <div className="section-header-actions">{actions}</div> : null}
    </div>
  );
}

export type KeyValueItem = {
  label: string;
  value: ReactNode;
  /** Render the value in a monospace font (addresses, identifiers). */
  mono?: boolean;
  /** Text to copy; shows a copy button next to the value. */
  copy?: string;
  hidden?: boolean;
};

/**
 * KeyValueList renders labelled values as a description list: labels in a
 * fixed column on wide screens, stacked above their values on narrow ones.
 */
export function KeyValueList({ items }: { items: KeyValueItem[] }) {
  return (
    <dl className="kv-list">
      {items
        .filter((i) => !i.hidden)
        .map((i) => (
          <div key={i.label} className="kv-row">
            <dt>{i.label}</dt>
            <dd>
              <span className={i.mono ? "mono kv-mono" : undefined}>{i.value}</span>
              {i.copy ? <CopyButton value={i.copy} label="Copy" className="btn btn-ghost btn-sm kv-copy" /> : null}
            </dd>
          </div>
        ))}
    </dl>
  );
}

/**
 * StatTile is one figure in a row of summary stats. With href, the whole tile
 * is a link.
 */
export function StatTile({
  label,
  value,
  hint,
  href,
  icon,
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  href?: string;
  icon?: ReactNode;
}) {
  const body = (
    <>
      <div className="stat-tile-label">
        {icon}
        {label}
      </div>
      <div className="stat-tile-value">{value}</div>
      {hint ? <div className="stat-tile-hint">{hint}</div> : null}
    </>
  );
  return href ? (
    <Link href={href} className="card stat-tile stat-tile-link">
      {body}
    </Link>
  ) : (
    <div className="card stat-tile">{body}</div>
  );
}
