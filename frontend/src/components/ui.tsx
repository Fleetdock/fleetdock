"use client";

import type { ReactNode } from "react";

export function Modal({
  open,
  onClose,
  title,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
}) {
  if (!open) return null;
  return (
    <div className="overlay" onClick={onClose}>
      <div className="card modal" style={{ padding: "1.25rem" }} onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between" style={{ marginBottom: "1rem" }}>
          <h3 className="text-base font-semibold">{title}</h3>
          <button className="btn btn-ghost btn-sm" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="field">
      <label className="label">{label}</label>
      {children}
    </div>
  );
}

const STATUS_CLASS: Record<string, string> = {
  online: "badge-green",
  active: "badge-green",
  running: "badge-green",
  succeeded: "badge-green",
  completed: "badge-green",
  pending: "badge-amber",
  creating: "badge-amber",
  provisioning: "badge-amber",
  locked: "badge-amber",
  draining: "badge-amber",
  migrating: "badge-amber",
  offline: "badge-gray",
  stopped: "badge-gray",
  canceled: "badge-gray",
  expired: "badge-gray",
  deleting: "badge-red",
  failed: "badge-red",
  error: "badge-red",
};

export function StatusBadge({ status }: { status: string }) {
  return (
    <span className={`badge ${STATUS_CLASS[status] ?? "badge-gray"}`}>
      <span className="dot" />
      {status}
    </span>
  );
}

export function Spinner() {
  return <span className="spin" aria-label="loading" />;
}

export function EmptyState({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="card" style={{ padding: "2.5rem", textAlign: "center" }}>
      <p className="font-medium">{title}</p>
      {hint ? <p className="muted text-sm" style={{ marginTop: ".35rem" }}>{hint}</p> : null}
    </div>
  );
}

// pageNumbers returns page buttons with ellipsis gaps: 1 … p-1 p p+1 … N.
function pageNumbers(page: number, pageCount: number): (number | "…")[] {
  const wanted = new Set<number>([1, 2, page - 1, page, page + 1, pageCount - 1, pageCount]);
  const pages = [...wanted].filter((p) => p >= 1 && p <= pageCount).sort((a, b) => a - b);
  const out: (number | "…")[] = [];
  let prev = 0;
  for (const p of pages) {
    if (prev && p - prev > 1) out.push("…");
    out.push(p);
    prev = p;
  }
  return out;
}

export function Pagination({
  page,
  pageCount,
  hasMore = false,
  onPage,
}: {
  page: number;
  pageCount: number;
  // hasMore keeps "next" enabled past an estimated pageCount.
  hasMore?: boolean;
  onPage: (page: number) => void;
}) {
  if (pageCount <= 1 && !hasMore) return null;
  return (
    <div className="flex items-center gap-1">
      <button className="btn btn-sm" disabled={page === 1} onClick={() => onPage(1)} aria-label="First page">
        «
      </button>
      <button className="btn btn-sm" disabled={page === 1} onClick={() => onPage(page - 1)} aria-label="Previous page">
        ‹
      </button>
      {pageNumbers(page, pageCount).map((p, i) =>
        p === "…" ? (
          <span key={`gap-${i}`} className="muted text-sm" style={{ padding: "0 .3rem" }}>…</span>
        ) : (
          <button
            key={p}
            className={`btn btn-sm${p === page ? " btn-primary" : ""}`}
            onClick={() => onPage(p)}
            disabled={p === page}
          >
            {p}
          </button>
        ),
      )}
      <button
        className="btn btn-sm"
        disabled={page >= pageCount && !hasMore}
        onClick={() => onPage(page + 1)}
        aria-label="Next page"
      >
        ›
      </button>
      <button className="btn btn-sm" disabled={page >= pageCount} onClick={() => onPage(pageCount)} aria-label="Last page">
        »
      </button>
    </div>
  );
}

export function ErrorText({ message }: { message?: string }) {
  if (!message) return null;
  return (
    <p className="text-sm" style={{ color: "var(--danger)", marginTop: ".25rem" }}>
      {message}
    </p>
  );
}
