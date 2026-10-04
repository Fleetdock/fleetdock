"use client";

import Link from "next/link";
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { X } from "lucide-react";

export type ToastKind = "success" | "error" | "info";

interface Toast {
  id: number;
  kind: ToastKind;
  message: string;
  action?: ToastAction;
}

/** ToastAction is an optional link shown in the toast, e.g. "View". */
export interface ToastAction {
  label: string;
  href: string;
}

interface ToastContextValue {
  push: (kind: ToastKind, message: string, action?: ToastAction) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const idRef = useRef(0);

  const remove = useCallback((id: number) => {
    setToasts((t) => t.filter((x) => x.id !== id));
  }, []);

  const push = useCallback(
    (kind: ToastKind, message: string, action?: ToastAction) => {
      const id = (idRef.current += 1);
      // Keep at most 4 on screen; errors stay longer so they can be read.
      setToasts((t) => [...t.slice(-3), { id, kind, message, action }]);
      setTimeout(() => remove(id), kind === "error" ? 9000 : 5000);
    },
    [remove],
  );

  const value = useMemo(() => ({ push }), [push]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toasts" aria-live="polite" role="status">
        {toasts.map((t) => (
          <div key={t.id} className={`card toast toast-${t.kind}`}>
            <span className="toast-dot" aria-hidden />
            <span style={{ flex: 1 }}>{t.message}</span>
            {t.action ? (
              <Link href={t.action.href} className="link" onClick={() => remove(t.id)}>
                {t.action.label}
              </Link>
            ) : null}
            <button type="button" className="toast-close" aria-label="Dismiss" onClick={() => remove(t.id)}>
              <X size={14} />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

// useToast returns the toast dispatcher. Outside a provider it is a no-op so
// components stay usable in isolation.
export function useToast(): ToastContextValue {
  return useContext(ToastContext) ?? { push: () => {} };
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
      const msg = err instanceof Error && err.message && err.message !== "unauthorized" ? err.message : fallback;
      push("error", msg);
    },
    [push],
  );
}
