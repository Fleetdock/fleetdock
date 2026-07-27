"use client";

import { useState, type ReactNode } from "react";
import { Copy } from "lucide-react";

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

/**
 * Detail renders one label/value row. Shared by the database overview and the
 * connectivity cards, which previously carried two copies of this markup.
 */
export function Detail({ label, value, mono }: { label: string; value: ReactNode; mono?: boolean }) {
  return (
    <div className="flex gap-3 text-sm" style={{ marginBottom: ".25rem" }}>
      <span className="muted" style={{ minWidth: "8rem", flexShrink: 0 }}>
        {label}
      </span>
      <span style={mono ? { fontFamily: "ui-monospace, monospace", wordBreak: "break-all" } : undefined}>{value}</span>
    </div>
  );
}

/**
 * copyText copies to the clipboard and reports whether it worked.
 *
 * navigator.clipboard is undefined on plain-HTTP non-localhost origins, which
 * is the normal Fleetdock deployment, so the async API alone silently fails
 * while the UI claims success. Falls back to a hidden textarea + execCommand.
 */
export async function copyText(value: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch {
    // Permission denied or the document is not focused; try the fallback.
  }

  try {
    const el = document.createElement("textarea");
    el.value = value;
    el.setAttribute("readonly", "");
    el.style.position = "fixed";
    el.style.opacity = "0";
    document.body.appendChild(el);
    el.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(el);
    return ok;
  } catch {
    return false;
  }
}

/** CopyButton copies a value and reflects whether the copy actually succeeded. */
export function CopyButton({
  value,
  label = "Copy",
  className = "btn btn-sm",
}: {
  value: string;
  label?: string;
  className?: string;
}) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");

  async function onClick() {
    const ok = await copyText(value);
    setState(ok ? "copied" : "failed");
    setTimeout(() => setState("idle"), 2000);
  }

  return (
    <button type="button" className={className} onClick={onClick}>
      <Copy size={14} /> {state === "copied" ? "Copied" : state === "failed" ? "Press ⌘C" : label}
    </button>
  );
}

/**
 * SecretReveal shows values that exist exactly once. Used for API tokens,
 * enrollment commands, and database credentials.
 */
export function SecretReveal({
  open,
  onClose,
  title,
  hint,
  items,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  hint?: string;
  items: { label: string; value: string; copyable?: boolean }[];
}) {
  const visible = items.filter((i) => i.value);
  return (
    <Modal open={open} onClose={onClose} title={title}>
      <p className="text-sm muted" style={{ marginBottom: ".75rem" }}>
        {hint ?? "Copy these values now — they will not be shown again."}
      </p>
      {visible.map((item) => (
        <div key={item.label} style={{ marginBottom: ".85rem" }}>
          <div className="muted text-sm" style={{ marginBottom: ".25rem" }}>
            {item.label}
          </div>
          <div
            className="card"
            style={{
              padding: ".6rem",
              fontFamily: "ui-monospace, monospace",
              fontSize: ".75rem",
              wordBreak: "break-all",
            }}
          >
            {item.value}
          </div>
          {item.copyable === false ? null : (
            <div style={{ marginTop: ".35rem" }}>
              <CopyButton value={item.value} label={`Copy ${item.label.toLowerCase()}`} />
            </div>
          )}
        </div>
      ))}
      <button className="btn btn-primary" style={{ marginTop: ".25rem" }} onClick={onClose}>
        Done
      </button>
    </Modal>
  );
}

/** ConfirmModal replaces window.confirm for destructive actions. */
export function ConfirmModal({
  open,
  title,
  message,
  confirmLabel = "Confirm",
  danger,
  busy,
  onConfirm,
  onCancel,
}: {
  open: boolean;
  title: string;
  message: ReactNode;
  confirmLabel?: string;
  danger?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <Modal open={open} onClose={onCancel} title={title}>
      <div className="text-sm" style={{ marginBottom: "1rem" }}>
        {message}
      </div>
      <div className="flex gap-2">
        <button className={danger ? "btn btn-danger" : "btn btn-primary"} disabled={busy} onClick={onConfirm}>
          {busy ? "Working…" : confirmLabel}
        </button>
        <button className="btn" disabled={busy} onClick={onCancel}>
          Cancel
        </button>
      </div>
    </Modal>
  );
}
