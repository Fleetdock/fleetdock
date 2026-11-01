"use client";

import { useEffect, useRef, type ReactNode } from "react";

/**
 * Popover is a small panel anchored under its trigger. It closes on Esc and
 * on a click outside, and hands focus back to the trigger when it closes from
 * the keyboard.
 */
export function Popover({
  open,
  onClose,
  trigger,
  children,
  label,
  align = "left",
  className,
}: {
  open: boolean;
  onClose: () => void;
  trigger: ReactNode;
  children: ReactNode;
  label: string;
  align?: "left" | "right";
  className?: string;
}) {
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (root.current && !root.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
        root.current?.querySelector<HTMLElement>("button")?.focus();
      }
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey, true);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey, true);
    };
  }, [open, onClose]);
  return (
    <div className="dbx-popover-root" ref={root}>
      {trigger}
      {open ? (
        <div className={`dbx-popover${align === "right" ? " is-right" : ""}${className ? ` ${className}` : ""}`} role="dialog" aria-label={label}>
          {children}
        </div>
      ) : null}
    </div>
  );
}
