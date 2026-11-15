"use client";

import Link from "next/link";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { CheckCircle2, Info, X, XCircle } from "lucide-react";

import { copyText } from "./ui/base";

export type ToastKind = "success" | "error" | "info";

interface Toast {
  id: number;
  kind: ToastKind;
  message: string;
  action?: ToastAction;
  ttl: number;
}

const ICON = { success: CheckCircle2, error: XCircle, info: Info } as const;

/** ToastAction is an optional link or button shown in the toast, e.g. "View" or "Undo". */
export type ToastAction =
  | { label: string; href: string }
  | { label: string; onClick: () => void };

interface ToastContextValue {
  push: (kind: ToastKind, message: string, action?: ToastAction) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const idRef = useRef(0);
  const timers = useRef(new Map<number, ReturnType<typeof setTimeout>>());

  const remove = useCallback((id: number) => {
    const t = timers.current.get(id);
    if (t) clearTimeout(t);
    timers.current.delete(id);
    setToasts((list) => list.filter((x) => x.id !== id));
  }, []);

  const schedule = useCallback(
    (id: number, ttl: number) => {
      const old = timers.current.get(id);
      if (old) clearTimeout(old);
      timers.current.set(
        id,
        setTimeout(() => remove(id), ttl),
      );
    },
    [remove],
  );

  // Hovering or focusing the toast stack pauses dismissal so a toast can be
  // read and its action reached; leaving restarts every timer.
  const stack = useRef<HTMLDivElement>(null);
  const hovered = useRef(false);
  const current = useRef<Toast[]>([]);
  useEffect(() => {
    current.current = toasts;
  }, [toasts]);
  const pause = useCallback(() => {
    timers.current.forEach((t) => clearTimeout(t));
  }, []);
  const resume = useCallback(() => {
    if (hovered.current || stack.current?.contains(document.activeElement)) return;
    current.current.forEach((t) => schedule(t.id, Math.min(t.ttl, 4000)));
  }, [schedule]);
  // Closing a toast from inside the stack removes the focused element
  // without a blur event, so restart the others' timers explicitly.
  const dismiss = useCallback(
    (id: number) => {
      remove(id);
      setTimeout(resume, 0);
    },
    [remove, resume],
  );

  const push = useCallback(
    (kind: ToastKind, message: string, action?: ToastAction) => {
      const id = (idRef.current += 1);
      // Errors, and toasts offering an action (Undo), stay longer.
      const ttl = kind === "error" || action ? 9000 : 5000;
      // Keep at most 4 on screen.
      setToasts((t) => [...t.slice(-3), { id, kind, message, action, ttl }]);
      schedule(id, ttl);
    },
    [schedule],
  );

  const value = useMemo(() => ({ push }), [push]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div
        ref={stack}
        className="toasts"
        onPointerEnter={() => {
          hovered.current = true;
          pause();
        }}
        onPointerLeave={() => {
          hovered.current = false;
          resume();
        }}
        onFocus={pause}
        onBlur={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget)) setTimeout(resume, 0);
        }}
      >
        {/* Errors interrupt (assertive); everything else waits its turn. */}
        <div className="contents" role="alert" aria-live="assertive">
          {toasts.filter((t) => t.kind === "error").map((t) => renderToast(t))}
        </div>
        <div className="contents" role="status" aria-live="polite">
          {toasts.filter((t) => t.kind !== "error").map((t) => renderToast(t))}
        </div>
      </div>
    </ToastContext.Provider>
  );

  function renderToast(t: Toast) {
    const Icon = ICON[t.kind];
    return (
      <div key={t.id} className={`card toast toast-${t.kind}`}>
        <Icon size={16} className="toast-icon" aria-hidden />
        <span className="flex-1 min-w-0">{t.message}</span>
        {t.action && "href" in t.action ? (
          <Link
            href={t.action.href}
            className="link"
            onClick={() => dismiss(t.id)}
          >
            {t.action.label}
          </Link>
        ) : t.action ? (
          <button
            type="button"
            className="link toast-action"
            onClick={() => {
              dismiss(t.id);
              (t.action as { onClick: () => void }).onClick();
            }}
          >
            {t.action.label}
          </button>
        ) : null}
        <button
          type="button"
          className="toast-close"
          aria-label="Dismiss"
          onClick={() => dismiss(t.id)}
        >
          <X size={14} />
        </button>
      </div>
    );
  }
}

// useToast returns the toast dispatcher. Outside a provider it is a no-op so
// components stay usable in isolation.
export function useToast(): ToastContextValue {
  return useContext(ToastContext) ?? { push: () => {} };
}

/**
 * useCopy returns a function that copies text and toasts the outcome, so a
 * copy that the browser refuses (plain HTTP on a LAN address) is not silent.
 */
export function useCopy(): (text: string, done: string) => void {
  const { push } = useToast();
  return useCallback(
    (text: string, done: string) =>
      void copyText(text).then((ok) => push(ok ? "success" : "error", ok ? done : "The browser did not allow copying")),
    [push],
  );
}

/**
 * useErrorToast returns an onError callback for fire-and-forget mutations
 * (`mutation.mutate(vars, { onError: toastError("…") })`), so a failure is
 * never silent. Mutations awaited with mutateAsync show errors inline instead.
 */
export function useErrorToast(): (fallback: string) => (err: unknown) => void {
  const { push } = useToast();
  return useCallback(
    (fallback: string) => (err: unknown) => {
      const msg =
        err instanceof Error && err.message && err.message !== "unauthorized"
          ? err.message
          : fallback;
      push("error", msg);
    },
    [push],
  );
}
