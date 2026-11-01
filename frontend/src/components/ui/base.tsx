"use client";

import { Check, Copy, X } from "lucide-react";
import {
  cloneElement,
  isValidElement,
  useEffect,
  useId,
  useRef,
  useState,
  type ReactElement,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";

import { Input, Select, Textarea } from "./controls";
import { HelpText } from "./tooltip";

export function Modal({
  open,
  onClose,
  title,
  children,
  wide,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  /** A wider dialog for forms with several columns. */
  wide?: boolean;
}) {
  const titleId = useId();
  const ref = useRef<HTMLDivElement>(null);

  // Accessibility: Esc closes, Tab stays inside the dialog, focus moves into it
  // on open and back to whatever opened it on close, and the page behind does
  // not scroll.
  useEffect(() => {
    if (!open) return;
    const opener = document.activeElement as HTMLElement | null;
    const node = ref.current;
    const focusables = () =>
      node
        ? Array.from(
            node.querySelectorAll<HTMLElement>(
              'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
            ),
          ).filter(
            (el) => el.offsetParent !== null || el === document.activeElement,
          )
        : [];
    // Prefer an explicitly autofocused field; otherwise the first control
    // after the close button.
    if (node && !node.contains(document.activeElement)) {
      const auto = node.querySelector<HTMLElement>("[autofocus]");
      const list = focusables();
      (auto ?? list[1] ?? list[0] ?? node).focus();
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
        return;
      }
      if (e.key !== "Tab") return;
      const list = focusables();
      if (list.length === 0) return;
      const first = list[0];
      const last = list[list.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prevOverflow;
      opener?.focus?.();
    };
    // onClose is intentionally not a dependency: re-running would steal focus
    // back on every parent render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  if (!open || typeof document === "undefined") return null;
  return createPortal(
    <div
      className="overlay"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div
        ref={ref}
        className={`card modal${wide ? " modal-wide" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
      >
        <div className="modal-head">
          <h2 id={titleId} className="modal-title">
            {title}
          </h2>
          <button
            type="button"
            className="btn btn-ghost btn-sm btn-icon"
            onClick={onClose}
            aria-label="Close dialog"
          >
            <X size={16} />
          </button>
        </div>
        {children}
      </div>
    </div>,
    document.body,
  );
}

// Controls Field links to its label: the raw elements and the primitives
// that render them (which forward id and aria-* props to the element).
const FORM_CONTROLS: unknown[] = ["input", "select", "textarea", Input, Select, Textarea];
function isFormControl(type: unknown): boolean {
  return FORM_CONTROLS.includes(type);
}

/**
 * Field labels a form control. When the child is a single input, select or
 * textarea it is linked to the label automatically (and to the hint/error
 * text), so screen readers announce it and clicking the label focuses it.
 */
export function Field({
  label,
  children,
  hint,
  error,
  help,
}: {
  label: ReactNode;
  children: ReactNode;
  /** Short guidance under the control. */
  hint?: ReactNode;
  /** Validation message for this field. */
  error?: string;
  /** Longer explanation behind an (i) icon next to the label. */
  help?: ReactNode;
}) {
  const id = useId();
  const hintId = hint ? `${id}-hint` : undefined;
  const errorId = error ? `${id}-error` : undefined;
  const describedBy = [hintId, errorId].filter(Boolean).join(" ") || undefined;

  let control = children;
  let linked = false;
  if (isValidElement(children) && isFormControl(children.type)) {
    const el = children as ReactElement<Record<string, unknown>>;
    linked = true;
    control = cloneElement(el, {
      id: (el.props.id as string) ?? id,
      "aria-describedby": describedBy,
      "aria-invalid": error ? true : undefined,
    });
  }
  const controlId =
    linked && isValidElement(control)
      ? ((control.props as { id?: string }).id ?? id)
      : undefined;

  return (
    <div className="field">
      <div className="flex items-center gap-1">
        <label className="label" htmlFor={controlId}>
          {label}
        </label>
        {help ? <HelpText>{help}</HelpText> : null}
      </div>
      {control}
      {hint ? (
        <p id={hintId} className="field-hint">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p id={errorId} className="field-error" role="alert">
          {error}
        </p>
      ) : null}
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
  missing: "badge-amber",
  paused: "badge-amber",
  canceling: "badge-amber",
  unknown: "badge-amber",
  skipped: "badge-gray",
};

// Plain-language labels and explanations for statuses whose raw value is
// unclear on its own.
const STATUS_LABEL: Record<string, string> = {
  missing: "not found on server",
  deleting: "removing",
  provisioning: "starting up",
  creating: "creating",
};

const STATUS_HELP: Record<string, string> = {
  missing:
    "Fleetdock no longer sees this database on its server. It may have been dropped or renamed outside Fleetdock; it comes back automatically if it reappears.",
  locked: "Writes are blocked until the database is unlocked.",
  migrating: "Being copied or moved; writes are paused.",
  offline: "No heartbeat from the agent for over two minutes.",
  pending: "Waiting to run.",
  error: "The last operation on this failed — see Activity for details.",
};

// Statuses that mean "something is happening right now": their dot pulses.
const STATUS_BUSY = new Set([
  "pending",
  "creating",
  "provisioning",
  "deleting",
  "migrating",
  "canceling",
  "running",
]);

export function StatusBadge({
  status,
  title,
}: {
  status: string;
  title?: string;
}) {
  const busy = STATUS_BUSY.has(status) && status !== "running";
  return (
    <span
      className={`badge ${STATUS_CLASS[status] ?? "badge-gray"}${busy ? " badge-pulse" : ""}`}
      title={title ?? STATUS_HELP[status]}
    >
      <span className="dot" aria-hidden />
      {STATUS_LABEL[status] ?? status}
    </span>
  );
}

export function Spinner({ label = "Loading" }: { label?: string }) {
  return <span className="spin" role="status" aria-label={label} />;
}

/**
 * EmptyState explains why a view is empty and, with `action`, offers the next
 * step — an empty page should never be a dead end.
 */
export function EmptyState({
  title,
  hint,
  action,
  icon,
  plain,
}: {
  title: string;
  hint?: ReactNode;
  action?: ReactNode;
  icon?: ReactNode;
  /** No card border — for use inside another card. */
  plain?: boolean;
}) {
  return (
    <div className={`card empty-state${plain ? " empty-state-plain" : ""}`}>
      {icon ? (
        <div className="empty-state-icon" aria-hidden>
          {icon}
        </div>
      ) : null}
      <p className="empty-state-title">{title}</p>
      {hint ? <p className="empty-state-hint">{hint}</p> : null}
      {action ? <div className="empty-state-actions">{action}</div> : null}
    </div>
  );
}

// pageNumbers returns page buttons with ellipsis gaps: 1 … p-1 p p+1 … N.
function pageNumbers(page: number, pageCount: number): (number | "…")[] {
  const wanted = new Set<number>([
    1,
    2,
    page - 1,
    page,
    page + 1,
    pageCount - 1,
    pageCount,
  ]);
  const pages = [...wanted]
    .filter((p) => p >= 1 && p <= pageCount)
    .sort((a, b) => a - b);
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
      <button
        className="btn btn-sm"
        disabled={page === 1}
        onClick={() => onPage(1)}
        aria-label="First page"
      >
        «
      </button>
      <button
        className="btn btn-sm"
        disabled={page === 1}
        onClick={() => onPage(page - 1)}
        aria-label="Previous page"
      >
        ‹
      </button>
      {pageNumbers(page, pageCount).map((p, i) =>
        p === "…" ? (
          <span key={`gap-${i}`} className="px-1 text-sm muted">
            …
          </span>
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
      <button
        className="btn btn-sm"
        disabled={page >= pageCount}
        onClick={() => onPage(pageCount)}
        aria-label="Last page"
      >
        »
      </button>
    </div>
  );
}

export function ErrorText({ message }: { message?: string }) {
  if (!message) return null;
  return (
    <p className="mt-1 text-danger text-sm" role="alert">
      {message}
    </p>
  );
}

/**
 * Detail renders one label/value row. Shared by the database overview and the
 * connectivity cards, which previously carried two copies of this markup.
 */
export function Detail({
  label,
  value,
  mono,
}: {
  label: string;
  value: ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="flex gap-3 mb-1.5 text-sm">
      <span className="min-w-32 muted shrink-0">{label}</span>
      <span className={mono ? "mono break-all" : "min-w-0"}>{value}</span>
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
    <button
      type="button"
      className={className}
      onClick={onClick}
      aria-live="polite"
    >
      {state === "copied" ? (
        <Check size={14} aria-hidden />
      ) : (
        <Copy size={14} aria-hidden />
      )}{" "}
      {state === "copied" ? "Copied" : state === "failed" ? "Press ⌘C" : label}
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
      <p className="mb-3 text-sm muted">
        {hint ?? "Copy these values now — they will not be shown again."}
      </p>
      {visible.map((item) => (
        <div key={item.label} className="mb-3.5">
          <div className="label">{item.label}</div>
          <div className="secret-value">{item.value}</div>
          {item.copyable === false ? null : (
            <div className="mt-1.5">
              <CopyButton
                value={item.value}
                label={`Copy ${item.label.toLowerCase()}`}
              />
            </div>
          )}
        </div>
      ))}
      <div className="modal-actions">
        <button type="button" className="btn btn-primary" onClick={onClose}>
          Done
        </button>
      </div>
    </Modal>
  );
}

/**
 * ConfirmModal replaces window.confirm for destructive actions. With
 * confirmText set, the confirm button stays disabled until the user types
 * that text — for actions that destroy data irreversibly.
 */
export function ConfirmModal({
  open,
  title,
  message,
  confirmLabel = "Confirm",
  confirmText,
  danger,
  busy,
  onConfirm,
  onCancel,
}: {
  open: boolean;
  title: string;
  message: ReactNode;
  confirmLabel?: string;
  confirmText?: string;
  danger?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const [typed, setTyped] = useState("");
  useEffect(() => {
    if (!open) setTyped("");
  }, [open]);
  const blocked = confirmText !== undefined && typed !== confirmText;
  return (
    <Modal open={open} onClose={onCancel} title={title}>
      <div className="mb-4 text-sm">{message}</div>
      {confirmText !== undefined ? (
        <Field label={`Type ${confirmText} to confirm`}>
          <input
            className="input"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            autoComplete="off"
            autoFocus
            aria-label="Confirmation text"
          />
        </Field>
      ) : null}
      <div className="modal-actions">
        <button
          type="button"
          className="btn"
          disabled={busy}
          onClick={onCancel}
        >
          Cancel
        </button>
        <button
          type="button"
          className={danger ? "btn btn-danger-solid" : "btn btn-primary"}
          disabled={busy || blocked}
          onClick={onConfirm}
        >
          {busy ? <span className="spin" aria-hidden /> : null}
          {busy ? "Working…" : confirmLabel}
        </button>
      </div>
    </Modal>
  );
}
