"use client";

import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from "react";

import { ConfirmModal } from "@/components/ui";

export type ConfirmOptions = {
  title: string;
  message: ReactNode;
  confirmLabel?: string;
  /** Red confirm button, for actions that destroy something. */
  danger?: boolean;
  /** Text the user must type to enable the confirm button. */
  confirmText?: string;
};

type ConfirmFn = (opts: ConfirmOptions) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFn | null>(null);

/**
 * ConfirmProvider renders one accessible confirmation dialog for the whole
 * app; useConfirm() opens it and resolves to the user's answer.
 */
export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [opts, setOpts] = useState<ConfirmOptions | null>(null);
  const resolver = useRef<((ok: boolean) => void) | null>(null);

  const confirm = useCallback<ConfirmFn>((o) => {
    resolver.current?.(false);
    setOpts(o);
    return new Promise<boolean>((resolve) => {
      resolver.current = resolve;
    });
  }, []);

  const answer = (ok: boolean) => {
    resolver.current?.(ok);
    resolver.current = null;
    setOpts(null);
  };

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <ConfirmModal
        open={opts !== null}
        title={opts?.title ?? ""}
        message={opts?.message ?? ""}
        confirmLabel={opts?.confirmLabel}
        confirmText={opts?.confirmText}
        danger={opts?.danger}
        onConfirm={() => answer(true)}
        onCancel={() => answer(false)}
      />
    </ConfirmContext.Provider>
  );
}

/** useConfirm returns `confirm(opts) => Promise<boolean>`. */
export function useConfirm(): ConfirmFn {
  const ctx = useContext(ConfirmContext);
  if (!ctx) throw new Error("useConfirm must be used inside ConfirmProvider");
  return ctx;
}
